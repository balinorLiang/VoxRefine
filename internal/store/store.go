package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"polyphone-checker/internal/glossary"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrEmailTaken = errors.New("email already registered")
	ErrConflict   = errors.New("conflict")
)

type Store struct {
	pool *pgxpool.Pool
}

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type Case struct {
	ID            string    `json:"id"`
	TermID        string    `json:"term_id,omitempty"`
	TermWord      string    `json:"term_word"`
	WrongText     string    `json:"wrong_text"`
	CorrectPinyin string    `json:"correct_pinyin"`
	Note          string    `json:"note"`
	Status        string    `json:"status"`
	Author        string    `json:"author"`
	Votes         int       `json:"votes"`
	Voted         bool      `json:"voted"`
	CreatedAt     time.Time `json:"created_at"`
}

type Stats struct {
	Terms        int `json:"terms"`
	Cases        int `json:"cases"`
	Pending      int `json:"pending"`
	Contributors int `json:"contributors"`
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) CountTerms(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM terms").Scan(&count)
	return count, err
}

func (s *Store) SeedGlossary(ctx context.Context, seed glossary.Seed, force bool) (int, error) {
	if !force {
		count, err := s.CountTerms(ctx)
		if err != nil {
			return 0, err
		}
		if count > 0 {
			return 0, nil
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, entry := range seed.Entries {
		pattern, contextAfter := glossary.NormalizePattern(entry.Word, entry.Pattern)
		if contextAfter == nil {
			contextAfter = []string{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO terms (id, word, pattern, context_after, type, risk, default_pinyin, note)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (id) DO UPDATE SET
				word = EXCLUDED.word,
				pattern = EXCLUDED.pattern,
				context_after = EXCLUDED.context_after,
				type = EXCLUDED.type,
				risk = EXCLUDED.risk,
				default_pinyin = EXCLUDED.default_pinyin,
				note = EXCLUDED.note,
				updated_at = now()`,
			entry.ID, entry.Word, pattern, contextAfter, entry.Type, entry.Risk, entry.Default, entry.Note,
		); err != nil {
			return 0, fmt.Errorf("upsert term %s: %w", entry.ID, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM readings WHERE term_id = $1", entry.ID); err != nil {
			return 0, fmt.Errorf("clear readings for %s: %w", entry.ID, err)
		}
		for index, reading := range entry.Readings {
			if _, err := tx.Exec(ctx, `
				INSERT INTO readings (term_id, pinyin, note, is_wrong, sort_order)
				VALUES ($1, $2, $3, $4, $5)`,
				entry.ID, reading.Pinyin, reading.Note, reading.Wrong, index,
			); err != nil {
				return 0, fmt.Errorf("insert reading for %s: %w", entry.ID, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(seed.Entries), nil
}

func (s *Store) LoadGlossary(ctx context.Context) ([]glossary.Entry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.word, t.pattern, t.context_after, t.type, t.risk, t.default_pinyin, t.note,
		       COALESCE(r.pinyin, ''), COALESCE(r.note, ''), COALESCE(r.is_wrong, false), COALESCE(r.sort_order, 0)
		FROM terms t
		LEFT JOIN readings r ON r.term_id = t.id
		ORDER BY t.id, r.sort_order, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]glossary.Entry, 0, 128)
	index := make(map[string]int, 128)
	for rows.Next() {
		var entry glossary.Entry
		var pinyin, note string
		var wrong bool
		var sortOrder int
		if err := rows.Scan(&entry.ID, &entry.Word, &entry.Pattern, &entry.ContextAfter, &entry.Type, &entry.Risk, &entry.Default, &entry.Note, &pinyin, &note, &wrong, &sortOrder); err != nil {
			return nil, err
		}
		position, ok := index[entry.ID]
		if !ok {
			entries = append(entries, entry)
			position = len(entries) - 1
			index[entry.ID] = position
		}
		if pinyin != "" {
			entries[position].Readings = append(entries[position].Readings, glossary.Reading{
				Pinyin: pinyin,
				Note:   note,
				Wrong:  wrong,
			})
		}
	}
	return entries, rows.Err()
}

func (s *Store) SearchTerms(ctx context.Context, query, termType, risk string, limit, offset int) ([]glossary.Entry, int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.word, t.pattern, t.context_after, t.type, t.risk, t.default_pinyin, t.note,
		       COALESCE(json_agg(json_build_object('pinyin', r.pinyin, 'note', r.note, 'wrong', r.is_wrong)
		                         ORDER BY r.sort_order, r.id)
		                FILTER (WHERE r.id IS NOT NULL), '[]'::json),
		       COUNT(*) OVER()
		FROM terms t
		LEFT JOIN readings r ON r.term_id = t.id
		WHERE ($1 = '' OR t.word ILIKE '%' || $1 || '%' OR t.note ILIKE '%' || $1 || '%'
		       OR EXISTS (
		           SELECT 1 FROM readings rx
		           WHERE rx.term_id = t.id
		             AND (rx.pinyin ILIKE '%' || $1 || '%' OR rx.note ILIKE '%' || $1 || '%')
		       ))
		  AND ($2 = '' OR t.type = $2)
		  AND ($3 = '' OR t.risk = $3)
		GROUP BY t.id
		ORDER BY CASE WHEN t.word = $1 THEN 0 ELSE 1 END,
		         CASE WHEN t.risk = 'high' THEN 0 ELSE 1 END,
		         t.word
		LIMIT $4 OFFSET $5`, query, termType, risk, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]glossary.Entry, 0, limit)
	total := 0
	for rows.Next() {
		var entry glossary.Entry
		var readingsJSON []byte
		if err := rows.Scan(&entry.ID, &entry.Word, &entry.Pattern, &entry.ContextAfter, &entry.Type, &entry.Risk, &entry.Default, &entry.Note, &readingsJSON, &total); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(readingsJSON, &entry.Readings); err != nil {
			return nil, 0, err
		}
		items = append(items, entry)
	}
	return items, total, rows.Err()
}

func (s *Store) GetTerm(ctx context.Context, id string) (glossary.Entry, error) {
	var entry glossary.Entry
	var readingsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT t.id, t.word, t.pattern, t.context_after, t.type, t.risk, t.default_pinyin, t.note,
		       COALESCE(json_agg(json_build_object('pinyin', r.pinyin, 'note', r.note, 'wrong', r.is_wrong)
		                         ORDER BY r.sort_order, r.id)
		                FILTER (WHERE r.id IS NOT NULL), '[]'::json)
		FROM terms t
		LEFT JOIN readings r ON r.term_id = t.id
		WHERE t.id = $1
		GROUP BY t.id`, id).Scan(&entry.ID, &entry.Word, &entry.Pattern, &entry.ContextAfter, &entry.Type, &entry.Risk, &entry.Default, &entry.Note, &readingsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return glossary.Entry{}, ErrNotFound
	}
	if err != nil {
		return glossary.Entry{}, err
	}
	if err := json.Unmarshal(readingsJSON, &entry.Readings); err != nil {
		return glossary.Entry{}, err
	}
	return entry, nil
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, displayName, role string) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, email, display_name, role, created_at`,
		email, passwordHash, displayName, role,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.CreatedAt)
	if isUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	return user, err
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, string, error) {
	var user User
	var passwordHash string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, display_name, role, created_at, password_hash
		FROM users WHERE lower(email) = lower($1)`, email,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return user, passwordHash, err
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, display_name, role, created_at
		FROM users WHERE id = $1::uuid`, id,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (s *Store) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)`, userID, tokenHash, expiresAt)
	return err
}

func (s *Store) UserBySession(ctx context.Context, tokenHash string) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.role, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1 AND last_seen_at < now() - interval '1 hour'`, tokenHash)
	return user, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE token_hash = $1", tokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE expires_at < now()")
	return err
}

func (s *Store) ListCases(ctx context.Context, status, viewerID, filterUserID string, limit, offset int) ([]Case, error) {
	if viewerID == "" {
		viewerID = "00000000-0000-0000-0000-000000000000"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id::text, COALESCE(c.term_id, ''), COALESCE(t.word, ''), c.wrong_text,
		       c.correct_pinyin, c.note, c.status, COALESCE(u.display_name, '匿名'), c.created_at,
		       (SELECT count(*) FROM case_votes v WHERE v.case_id = c.id),
		       EXISTS (SELECT 1 FROM case_votes v2 WHERE v2.case_id = c.id AND v2.user_id = $1::uuid)
		FROM cases c
		LEFT JOIN terms t ON t.id = c.term_id
		LEFT JOIN users u ON u.id = c.user_id
		WHERE ($2 = '' OR c.status = $2)
		  AND ($3 = '' OR c.user_id = NULLIF($3, '')::uuid)
		ORDER BY c.created_at DESC
		LIMIT $4 OFFSET $5`, viewerID, status, filterUserID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Case, 0, limit)
	for rows.Next() {
		var item Case
		if err := rows.Scan(&item.ID, &item.TermID, &item.TermWord, &item.WrongText, &item.CorrectPinyin, &item.Note, &item.Status, &item.Author, &item.CreatedAt, &item.Votes, &item.Voted); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateCase(ctx context.Context, userID, termID, wrongText, correctPinyin, note string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO cases (user_id, term_id, wrong_text, correct_pinyin, note)
		VALUES ($1::uuid, NULLIF($2, ''), $3, $4, $5)
		RETURNING id::text`, userID, termID, wrongText, correctPinyin, note,
	).Scan(&id)
	if isForeignKeyViolation(err) {
		return "", ErrNotFound
	}
	return id, err
}

func (s *Store) VoteCase(ctx context.Context, caseID, userID string) (int, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM cases WHERE id = $1::uuid AND status = 'approved')", caseID).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrNotFound
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO case_votes (case_id, user_id) VALUES ($1::uuid, $2::uuid)
		ON CONFLICT DO NOTHING`, caseID, userID); err != nil {
		return 0, err
	}
	var votes int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM case_votes WHERE case_id = $1::uuid", caseID).Scan(&votes)
	return votes, err
}

func (s *Store) UnvoteCase(ctx context.Context, caseID, userID string) (int, error) {
	if _, err := s.pool.Exec(ctx, "DELETE FROM case_votes WHERE case_id = $1::uuid AND user_id = $2::uuid", caseID, userID); err != nil {
		return 0, err
	}
	var votes int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM case_votes WHERE case_id = $1::uuid", caseID).Scan(&votes)
	return votes, err
}

func (s *Store) ModerateCase(ctx context.Context, caseID, moderatorID, status, note string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE cases
		SET status = $2, moderation_note = $3, moderated_by = $4::uuid, moderated_at = now()
		WHERE id = $1::uuid`, caseID, status, note, moderatorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM terms),
		       (SELECT count(*) FROM cases WHERE status = 'approved'),
		       (SELECT count(*) FROM cases WHERE status = 'pending'),
		       (SELECT count(DISTINCT user_id) FROM cases WHERE user_id IS NOT NULL)`).
		Scan(&stats.Terms, &stats.Cases, &stats.Pending, &stats.Contributors)
	return stats, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
