package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"polyphone-checker/internal/database"
	"polyphone-checker/internal/glossary"
	"polyphone-checker/internal/store"
	"polyphone-checker/migrations"
)

func newIntegrationStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)

	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := fmt.Sprintf("polyphone_test_%d_%s", time.Now().UnixNano(), hex.EncodeToString(random[:]))
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		if _, err := adminPool.Exec(dropCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	config.MaxConns = 4
	config.MinConns = 0
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["search_path"] = identifier + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect test pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}
	if err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	return store.New(pool), ctx
}

func integrationSeed() glossary.Seed {
	return glossary.Seed{
		Version: "test",
		Entries: []glossary.Entry{
			{
				ID:      "chongqing",
				Word:    "重庆",
				Pattern: "重庆",
				Type:    "place",
				Risk:    "high",
				Default: "chóng qìng",
				Note:    "直辖市",
				Readings: []glossary.Reading{
					{Pinyin: "chóng qìng", Note: "正确读音"},
					{Pinyin: "zhòng qìng", Wrong: true},
				},
			},
			{
				ID:       "yinhang",
				Word:     "银行",
				Type:     "word",
				Risk:     "high",
				Default:  "yín háng",
				Readings: []glossary.Reading{{Pinyin: "yín háng"}},
			},
			{
				ID:       "zeng",
				Word:     "曾",
				Pattern:  "曾(?=先生|女士)",
				Type:     "surname",
				Risk:     "medium",
				Default:  "zēng",
				Readings: []glossary.Reading{{Pinyin: "zēng"}},
			},
		},
	}
}

func TestStoreIntegrationMigrationsAndGlossary(t *testing.T) {
	repo, ctx := newIntegrationStore(t)

	count, err := repo.CountTerms(ctx)
	if err != nil || count != 0 {
		t.Fatalf("initial CountTerms = %d, %v", count, err)
	}

	seeded, err := repo.SeedGlossary(ctx, integrationSeed(), false)
	if err != nil || seeded != 3 {
		t.Fatalf("SeedGlossary = %d, %v", seeded, err)
	}
	seeded, err = repo.SeedGlossary(ctx, integrationSeed(), false)
	if err != nil || seeded != 0 {
		t.Fatalf("second SeedGlossary = %d, %v", seeded, err)
	}

	entries, err := repo.LoadGlossary(ctx)
	if err != nil || len(entries) != 3 {
		t.Fatalf("LoadGlossary = %d, %v", len(entries), err)
	}
	byID := make(map[string]glossary.Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	if got := byID["zeng"].ContextAfter; len(got) != 2 || got[0] != "先生" || got[1] != "女士" {
		t.Fatalf("zeng ContextAfter = %#v", got)
	}
	if got := byID["chongqing"].Readings; len(got) != 2 || !got[1].Wrong {
		t.Fatalf("chongqing readings = %#v", got)
	}

	items, total, err := repo.SearchTerms(ctx, "银行", "word", "high", 10, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != "yinhang" {
		t.Fatalf("SearchTerms by word = %#v total=%d err=%v", items, total, err)
	}
	items, total, err = repo.SearchTerms(ctx, "chóng", "", "", 10, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != "chongqing" {
		t.Fatalf("SearchTerms by pinyin = %#v total=%d err=%v", items, total, err)
	}
	items, total, err = repo.SearchTerms(ctx, "", "", "", 2, 1)
	if err != nil || total != 3 || len(items) != 2 {
		t.Fatalf("SearchTerms pagination = %#v total=%d err=%v", items, total, err)
	}
	items, total, err = repo.SearchTerms(ctx, "missing", "", "", 10, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("SearchTerms missing = %#v total=%d err=%v", items, total, err)
	}

	updated := integrationSeed()
	updated.Entries[0].Note = "更新后的说明"
	updated.Entries[0].Readings = []glossary.Reading{{Pinyin: "chóng qìng"}}
	seeded, err = repo.SeedGlossary(ctx, updated, true)
	if err != nil || seeded != 3 {
		t.Fatalf("forced SeedGlossary = %d, %v", seeded, err)
	}
	entry, err := repo.GetTerm(ctx, "chongqing")
	if err != nil || entry.Note != "更新后的说明" || len(entry.Readings) != 1 {
		t.Fatalf("GetTerm after force = %#v, %v", entry, err)
	}
	if _, err := repo.GetTerm(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetTerm missing error = %v", err)
	}
}

func TestStoreIntegrationAccountsAndSessions(t *testing.T) {
	repo, ctx := newIntegrationStore(t)

	author, err := repo.CreateUser(ctx, "Editor@Example.com", "password-hash", "编辑者", "user")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	byEmail, passwordHash, err := repo.GetUserByEmail(ctx, "editor@EXAMPLE.com")
	if err != nil || byEmail.ID != author.ID || passwordHash != "password-hash" {
		t.Fatalf("GetUserByEmail = %#v hash=%q err=%v", byEmail, passwordHash, err)
	}
	if _, err := repo.CreateUser(ctx, "EDITOR@example.com", "other", "重复账号", "user"); !errors.Is(err, store.ErrEmailTaken) {
		t.Fatalf("duplicate CreateUser error = %v", err)
	}
	byID, err := repo.GetUserByID(ctx, author.ID)
	if err != nil || byID.Email != author.Email || byID.Role != "user" {
		t.Fatalf("GetUserByID = %#v, %v", byID, err)
	}
	if _, err := repo.GetUserByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing GetUserByID error = %v", err)
	}
	if _, _, err := repo.GetUserByEmail(ctx, "missing@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing GetUserByEmail error = %v", err)
	}

	activeToken := "active-session-token-hash"
	if err := repo.CreateSession(ctx, author.ID, activeToken, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessionUser, err := repo.UserBySession(ctx, activeToken)
	if err != nil || sessionUser.ID != author.ID {
		t.Fatalf("UserBySession = %#v, %v", sessionUser, err)
	}

	expiredToken := "expired-session-token-hash"
	if err := repo.CreateSession(ctx, author.ID, expiredToken, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}
	if _, err := repo.UserBySession(ctx, expiredToken); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired UserBySession error = %v", err)
	}
	if err := repo.DeleteExpiredSessions(ctx); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	var expiredCount int
	if err := repo.Pool().QueryRow(ctx, "SELECT count(*) FROM sessions WHERE token_hash = $1", expiredToken).Scan(&expiredCount); err != nil {
		t.Fatalf("count expired sessions: %v", err)
	}
	if expiredCount != 0 {
		t.Fatalf("expired sessions remaining = %d", expiredCount)
	}

	if err := repo.DeleteSession(ctx, activeToken); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := repo.UserBySession(ctx, activeToken); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted UserBySession error = %v", err)
	}
}

func TestStoreIntegrationCasesVotesModerationAndStats(t *testing.T) {
	repo, ctx := newIntegrationStore(t)
	if _, err := repo.SeedGlossary(ctx, integrationSeed(), false); err != nil {
		t.Fatalf("SeedGlossary: %v", err)
	}

	author, err := repo.CreateUser(ctx, "author@example.com", "hash", "作者甲", "user")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	secondAuthor, err := repo.CreateUser(ctx, "second@example.com", "hash", "作者乙", "user")
	if err != nil {
		t.Fatalf("CreateUser second author: %v", err)
	}
	moderator, err := repo.CreateUser(ctx, "moderator@example.com", "hash", "审核员", "moderator")
	if err != nil {
		t.Fatalf("CreateUser moderator: %v", err)
	}

	caseID, err := repo.CreateCase(ctx, author.ID, "chongqing", "重（zhòng）庆", "chóng qìng", "语境说明")
	if err != nil {
		t.Fatalf("CreateCase: %v", err)
	}
	if _, err := repo.CreateCase(ctx, author.ID, "missing-term", "错误", "正确", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("CreateCase missing term error = %v", err)
	}

	pending, err := repo.ListCases(ctx, "pending", author.ID, author.ID, 10, 0)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending ListCases = %#v, %v", pending, err)
	}
	if pending[0].ID != caseID || pending[0].Author != "作者甲" || pending[0].TermWord != "重庆" || pending[0].Votes != 0 || pending[0].Voted {
		t.Fatalf("pending case = %#v", pending[0])
	}
	if _, err := repo.VoteCase(ctx, caseID, secondAuthor.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("VoteCase pending error = %v", err)
	}

	if err := repo.ModerateCase(ctx, caseID, moderator.ID, "approved", "通过"); err != nil {
		t.Fatalf("ModerateCase: %v", err)
	}
	approved, err := repo.ListCases(ctx, "approved", author.ID, "", 10, 0)
	if err != nil || len(approved) != 1 || approved[0].Status != "approved" || approved[0].Voted {
		t.Fatalf("approved ListCases = %#v, %v", approved, err)
	}

	votes, err := repo.VoteCase(ctx, caseID, author.ID)
	if err != nil || votes != 1 {
		t.Fatalf("first VoteCase = %d, %v", votes, err)
	}
	votes, err = repo.VoteCase(ctx, caseID, author.ID)
	if err != nil || votes != 1 {
		t.Fatalf("duplicate VoteCase = %d, %v", votes, err)
	}
	approved, err = repo.ListCases(ctx, "approved", author.ID, "", 10, 0)
	if err != nil || len(approved) != 1 || !approved[0].Voted || approved[0].Votes != 1 {
		t.Fatalf("voted ListCases = %#v, %v", approved, err)
	}
	votes, err = repo.VoteCase(ctx, caseID, secondAuthor.ID)
	if err != nil || votes != 2 {
		t.Fatalf("second user VoteCase = %d, %v", votes, err)
	}
	votes, err = repo.UnvoteCase(ctx, caseID, author.ID)
	if err != nil || votes != 1 {
		t.Fatalf("UnvoteCase = %d, %v", votes, err)
	}
	votes, err = repo.UnvoteCase(ctx, caseID, author.ID)
	if err != nil || votes != 1 {
		t.Fatalf("duplicate UnvoteCase = %d, %v", votes, err)
	}

	secondCaseID, err := repo.CreateCase(ctx, secondAuthor.ID, "", "错误文本", "正确拼音", "")
	if err != nil {
		t.Fatalf("CreateCase without term: %v", err)
	}
	if err := repo.ModerateCase(ctx, secondCaseID, moderator.ID, "rejected", "不采纳"); err != nil {
		t.Fatalf("reject case: %v", err)
	}
	if err := repo.ModerateCase(ctx, "00000000-0000-0000-0000-000000000000", moderator.ID, "approved", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing ModerateCase error = %v", err)
	}

	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Terms != 3 || stats.Cases != 1 || stats.Pending != 0 || stats.Contributors != 2 {
		t.Fatalf("Stats = %#v", stats)
	}
}
