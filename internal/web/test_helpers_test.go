package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"polyphone-checker/internal/config"
	"polyphone-checker/internal/glossary"
	"polyphone-checker/internal/store"
)

var _ DataStore = (*fakeStore)(nil)

type fakeUser struct {
	User         store.User
	PasswordHash string
}

type fakeCase struct {
	Case   store.Case
	UserID string
}

type fakeStore struct {
	mu sync.Mutex

	pingErr          error
	searchErr        error
	getTermErr       error
	statsErr         error
	createUserErr    error
	getUserErr       error
	createSessionErr error
	userBySessionErr error
	deleteSessionErr error
	listCasesErr     error
	createCaseErr    error
	voteErr          error
	unvoteErr        error
	moderateErr      error

	terms     map[string]glossary.Entry
	termOrder []string
	users     map[string]fakeUser
	sessions  map[string]store.User
	cases     map[string]*fakeCase
	caseOrder []string
	votes     map[string]map[string]bool
	nextUser  int
	nextCase  int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		terms:    make(map[string]glossary.Entry),
		users:    make(map[string]fakeUser),
		sessions: make(map[string]store.User),
		cases:    make(map[string]*fakeCase),
		votes:    make(map[string]map[string]bool),
	}
}

func (f *fakeStore) addTerm(entry glossary.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.terms[entry.ID]; !exists {
		f.termOrder = append(f.termOrder, entry.ID)
	}
	f.terms[entry.ID] = entry
}

func (f *fakeStore) addUser(email, password, role string) store.User {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextUser++
	user := store.User{
		ID:          fmt.Sprintf("user-%d", f.nextUser),
		Email:       strings.ToLower(email),
		DisplayName: fmt.Sprintf("User %d", f.nextUser),
		Role:        role,
		CreatedAt:   time.Unix(1_700_000_000, 0).UTC(),
	}
	f.users[user.Email] = fakeUser{User: user, PasswordHash: string(hash)}
	return user
}

func (f *fakeStore) addSession(user store.User) *http.Cookie {
	token := "test-token-" + user.ID
	hash := hashSessionToken(token)
	f.mu.Lock()
	f.sessions[hash] = user
	f.mu.Unlock()
	return &http.Cookie{Name: "pc_session", Value: token}
}

func (f *fakeStore) addCase(userID, termID, wrong, pinyin, note, status string) store.Case {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextCase++
	id := fmt.Sprintf("case-%d", f.nextCase)
	entry := f.terms[termID]
	item := &fakeCase{
		UserID: userID,
		Case: store.Case{
			ID:            id,
			TermID:        termID,
			TermWord:      entry.Word,
			WrongText:     wrong,
			CorrectPinyin: pinyin,
			Note:          note,
			Status:        status,
			Author:        "Test User",
			CreatedAt:     time.Unix(1_700_000_000+int64(f.nextCase), 0).UTC(),
		},
	}
	f.cases[id] = item
	f.caseOrder = append(f.caseOrder, id)
	return item.Case
}

func (f *fakeStore) Ping(context.Context) error {
	return f.pingErr
}

func (f *fakeStore) SearchTerms(_ context.Context, query, termType, risk string, limit, offset int) ([]glossary.Entry, int, error) {
	if f.searchErr != nil {
		return nil, 0, f.searchErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]glossary.Entry, 0, len(f.termOrder))
	for _, id := range f.termOrder {
		entry := f.terms[id]
		if query != "" && !termMatches(entry, query) {
			continue
		}
		if termType != "" && entry.Type != termType {
			continue
		}
		if risk != "" && entry.Risk != risk {
			continue
		}
		items = append(items, entry)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Risk != items[j].Risk {
			return items[i].Risk == "high"
		}
		return items[i].Word < items[j].Word
	})
	total := len(items)
	if offset >= len(items) {
		return []glossary.Entry{}, total, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], total, nil
}

func termMatches(entry glossary.Entry, query string) bool {
	lower := strings.ToLower(query)
	if strings.Contains(strings.ToLower(entry.Word), lower) || strings.Contains(strings.ToLower(entry.Note), lower) {
		return true
	}
	for _, reading := range entry.Readings {
		if strings.Contains(strings.ToLower(reading.Pinyin), lower) || strings.Contains(strings.ToLower(reading.Note), lower) {
			return true
		}
	}
	return false
}

func (f *fakeStore) GetTerm(_ context.Context, id string) (glossary.Entry, error) {
	if f.getTermErr != nil {
		return glossary.Entry{}, f.getTermErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.terms[id]
	if !ok {
		return glossary.Entry{}, store.ErrNotFound
	}
	return entry, nil
}

func (f *fakeStore) Stats(context.Context) (store.Stats, error) {
	if f.statsErr != nil {
		return store.Stats{}, f.statsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	stats := store.Stats{Terms: len(f.terms)}
	contributors := make(map[string]bool)
	for _, item := range f.cases {
		switch item.Case.Status {
		case "approved":
			stats.Cases++
		case "pending":
			stats.Pending++
		}
		if item.UserID != "" {
			contributors[item.UserID] = true
		}
	}
	stats.Contributors = len(contributors)
	return stats, nil
}

func (f *fakeStore) CreateUser(_ context.Context, email, passwordHash, displayName, role string) (store.User, error) {
	if f.createUserErr != nil {
		return store.User{}, f.createUserErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.ToLower(email)
	if _, exists := f.users[key]; exists {
		return store.User{}, store.ErrEmailTaken
	}
	f.nextUser++
	user := store.User{
		ID:          fmt.Sprintf("user-%d", f.nextUser),
		Email:       email,
		DisplayName: displayName,
		Role:        role,
		CreatedAt:   time.Unix(1_700_000_000, 0).UTC(),
	}
	f.users[key] = fakeUser{User: user, PasswordHash: passwordHash}
	return user, nil
}

func (f *fakeStore) GetUserByEmail(_ context.Context, email string) (store.User, string, error) {
	if f.getUserErr != nil {
		return store.User{}, "", f.getUserErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.users[strings.ToLower(email)]
	if !ok {
		return store.User{}, "", store.ErrNotFound
	}
	return item.User, item.PasswordHash, nil
}

func (f *fakeStore) CreateSession(_ context.Context, userID, tokenHash string, _ time.Time) error {
	if f.createSessionErr != nil {
		return f.createSessionErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, item := range f.users {
		if item.User.ID == userID {
			f.sessions[tokenHash] = item.User
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeStore) UserBySession(_ context.Context, tokenHash string) (store.User, error) {
	if f.userBySessionErr != nil {
		return store.User{}, f.userBySessionErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	user, ok := f.sessions[tokenHash]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return user, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, tokenHash string) error {
	if f.deleteSessionErr != nil {
		return f.deleteSessionErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, tokenHash)
	return nil
}

func (f *fakeStore) ListCases(_ context.Context, status, viewerID, filterUserID string, limit, offset int) ([]store.Case, error) {
	if f.listCasesErr != nil {
		return nil, f.listCasesErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]store.Case, 0, len(f.caseOrder))
	for index := len(f.caseOrder) - 1; index >= 0; index-- {
		item := f.cases[f.caseOrder[index]]
		if status != "" && item.Case.Status != status {
			continue
		}
		if filterUserID != "" && item.UserID != filterUserID {
			continue
		}
		value := item.Case
		value.Votes = len(f.votes[value.ID])
		value.Voted = viewerID != "" && f.votes[value.ID][viewerID]
		items = append(items, value)
	}
	if offset >= len(items) {
		return []store.Case{}, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}

func (f *fakeStore) CreateCase(_ context.Context, userID, termID, wrongText, correctPinyin, note string) (string, error) {
	if f.createCaseErr != nil {
		return "", f.createCaseErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if termID != "" {
		if _, ok := f.terms[termID]; !ok {
			return "", store.ErrNotFound
		}
	}
	item := f.addCaseLocked(userID, termID, wrongText, correctPinyin, note, "pending")
	return item.ID, nil
}

func (f *fakeStore) addCaseLocked(userID, termID, wrong, pinyin, note, status string) store.Case {
	f.nextCase++
	id := fmt.Sprintf("case-%d", f.nextCase)
	entry := f.terms[termID]
	item := &fakeCase{
		UserID: userID,
		Case: store.Case{
			ID:            id,
			TermID:        termID,
			TermWord:      entry.Word,
			WrongText:     wrong,
			CorrectPinyin: pinyin,
			Note:          note,
			Status:        status,
			Author:        "Test User",
			CreatedAt:     time.Unix(1_700_000_000+int64(f.nextCase), 0).UTC(),
		},
	}
	f.cases[id] = item
	f.caseOrder = append(f.caseOrder, id)
	return item.Case
}

func (f *fakeStore) VoteCase(_ context.Context, caseID, userID string) (int, error) {
	if f.voteErr != nil {
		return 0, f.voteErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.cases[caseID]
	if !ok || item.Case.Status != "approved" {
		return 0, store.ErrNotFound
	}
	if f.votes[caseID] == nil {
		f.votes[caseID] = make(map[string]bool)
	}
	f.votes[caseID][userID] = true
	return len(f.votes[caseID]), nil
}

func (f *fakeStore) UnvoteCase(_ context.Context, caseID, userID string) (int, error) {
	if f.unvoteErr != nil {
		return 0, f.unvoteErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.votes[caseID] != nil {
		delete(f.votes[caseID], userID)
	}
	return len(f.votes[caseID]), nil
}

func (f *fakeStore) ModerateCase(_ context.Context, caseID, _ string, status, note string) error {
	if f.moderateErr != nil {
		return f.moderateErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.cases[caseID]
	if !ok {
		return store.ErrNotFound
	}
	item.Case.Status = status
	item.Case.Note = note
	return nil
}

func testConfig() config.Config {
	return config.Config{
		Env:           "test",
		Addr:          "127.0.0.1:0",
		AdminEmail:    "admin@example.com",
		SessionTTL:    time.Hour,
		CookieSecure:  false,
		TrustProxy:    false,
		SessionCookie: "pc_session",
	}
}

func testGlossaryEntries() []glossary.Entry {
	return []glossary.Entry{
		{
			ID: "chongqing", Word: "重庆", Type: "place", Risk: "high", Default: "chong2 qing4",
			Readings: []glossary.Reading{
				{Pinyin: "chong2 qing4", Note: "地名"},
				{Pinyin: "zhong4 qing4", Note: "误读", Wrong: true},
			},
			Note: "重庆读 chóng qìng",
		},
		{
			ID: "yinhang", Word: "银行", Type: "word", Risk: "high", Default: "yin2 hang2",
			Readings: []glossary.Reading{{Pinyin: "yin2 hang2", Note: "金融机构", Wrong: false}},
			Note:     "银行读 yín háng",
		},
	}
}

func newTestServer(t *testing.T, data *fakeStore, cfg config.Config, entries ...glossary.Entry) *Server {
	t.Helper()
	server := &Server{
		cfg:          cfg,
		store:        data,
		detector:     glossary.NewDetector(entries),
		mux:          http.NewServeMux(),
		limiter:      newRateLimiter(10_000, 10_000),
		authLimiter:  newRateLimiter(10_000, 10_000),
		checkLimiter: newRateLimiter(10_000, 10_000),
		writeLimiter: newRateLimiter(10_000, 10_000),
		dummyHash:    []byte("dummy-hash"),
		version:      "test-version",
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	server.routes()
	return server
}

func performRequest(t *testing.T, handler http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://example.com"+path, strings.NewReader(body))
	req.Host = "example.com"
	req.RemoteAddr = "192.0.2.1:12345"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func decodeJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
}
