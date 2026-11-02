package web

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"polyphone-checker/internal/glossary"
	"polyphone-checker/internal/store"
)

type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

type userResponse struct {
	User *store.User `json:"user"`
}

func TestCheckEndpoint(t *testing.T) {
	server := newTestServer(t, newFakeStore(), testConfig(), testGlossaryEntries()...)
	handler := server.Handler()

	recorder := performRequest(t, handler, http.MethodPost, "/api/check", mustJSON(t, map[string]any{"text": "欢迎来到重庆，银行行长出差了。"}))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Matches    []glossary.Match `json:"matches"`
		Total      int              `json:"total"`
		Truncated  bool             `json:"truncated"`
		TextLength int              `json:"text_length"`
		Engine     string           `json:"engine"`
	}
	decodeJSON(t, recorder, &response)
	if response.Total != 2 || len(response.Matches) != 2 || response.Truncated {
		t.Fatalf("check response = %#v", response)
	}
	if response.Matches[0].Text != "重庆" || response.Matches[0].Start != 4 || response.Matches[0].End != 6 {
		t.Fatalf("first match = %#v", response.Matches[0])
	}
	if response.TextLength != 15 || response.Engine != "glossary-v2" {
		t.Fatalf("metadata = %#v", response)
	}

	recorder = performRequest(t, handler, http.MethodPost, "/api/check", mustJSON(t, map[string]any{"text": "重庆重庆", "max_matches": 1}))
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.Total != 1 || !response.Truncated {
		t.Fatalf("truncated response = %d %#v", recorder.Code, response)
	}

	tests := []struct {
		name string
		body string
		code int
		err  string
	}{
		{name: "empty", body: `{"text":"   "}`, code: http.StatusBadRequest, err: "empty_text"},
		{name: "unknown field", body: `{"text":"重庆","extra":1}`, code: http.StatusBadRequest, err: "bad_request"},
		{name: "invalid json", body: `{"text":`, code: http.StatusBadRequest, err: "bad_request"},
		{name: "too long", body: mustJSON(t, map[string]any{"text": strings.Repeat("a", 200_001)}), code: http.StatusRequestEntityTooLarge, err: "text_too_long"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := performRequest(t, handler, http.MethodPost, "/api/check", test.body)
			if recorder.Code != test.code {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
			var body errorResponse
			decodeJSON(t, recorder, &body)
			if body.Code != test.err {
				t.Fatalf("code = %q, want %q", body.Code, test.err)
			}
		})
	}
}

func TestTermsEndpoints(t *testing.T) {
	data := newFakeStore()
	for _, entry := range testGlossaryEntries() {
		data.addTerm(entry)
	}
	server := newTestServer(t, data, testConfig(), testGlossaryEntries()...)
	handler := server.Handler()

	var response struct {
		Items  []glossary.Entry `json:"items"`
		Total  int              `json:"total"`
		Query  string           `json:"query"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	recorder := performRequest(t, handler, http.MethodGet, "/api/terms?q=%E9%87%8D%E5%BA%86", "")
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.Total != 1 || response.Items[0].ID != "chongqing" {
		t.Fatalf("search response = %d %#v", recorder.Code, response)
	}

	recorder = performRequest(t, handler, http.MethodGet, "/api/terms?q=chong2", "")
	decodeJSON(t, recorder, &response)
	if response.Total != 1 || response.Items[0].ID != "chongqing" {
		t.Fatalf("pinyin search = %#v", response)
	}

	recorder = performRequest(t, handler, http.MethodGet, "/api/terms?type=place&limit=999", "")
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.Total != 1 || response.Limit != 100 {
		t.Fatalf("filter response = %d %#v", recorder.Code, response)
	}

	for _, test := range []struct {
		path string
		code int
		err  string
	}{
		{path: "/api/terms?type=invalid", code: http.StatusBadRequest, err: "bad_type"},
		{path: "/api/terms?risk=invalid", code: http.StatusBadRequest, err: "bad_risk"},
		{path: "/api/terms?q=" + strings.Repeat("a", 101), code: http.StatusBadRequest, err: "query_too_long"},
	} {
		recorder := performRequest(t, handler, http.MethodGet, test.path, "")
		var body errorResponse
		decodeJSON(t, recorder, &body)
		if recorder.Code != test.code || body.Code != test.err {
			t.Fatalf("%s = %d %#v", test.path, recorder.Code, body)
		}
	}

	data.searchErr = errors.New("search failed")
	recorder = performRequest(t, handler, http.MethodGet, "/api/terms", "")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("search error status = %d", recorder.Code)
	}
}

func TestTermEndpoint(t *testing.T) {
	data := newFakeStore()
	for _, entry := range testGlossaryEntries() {
		data.addTerm(entry)
	}
	server := newTestServer(t, data, testConfig(), testGlossaryEntries()...)
	handler := server.Handler()

	recorder := performRequest(t, handler, http.MethodGet, "/api/terms/chongqing", "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "重庆") {
		t.Fatalf("term response = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = performRequest(t, handler, http.MethodGet, "/api/terms/missing", "")
	var body errorResponse
	decodeJSON(t, recorder, &body)
	if recorder.Code != http.StatusNotFound || body.Code != "not_found" {
		t.Fatalf("missing term = %d %#v", recorder.Code, body)
	}

	data.getTermErr = errors.New("query failed")
	recorder = performRequest(t, handler, http.MethodGet, "/api/terms/chongqing", "")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("term error status = %d", recorder.Code)
	}
}

func TestStatsEndpoint(t *testing.T) {
	data := newFakeStore()
	for _, entry := range testGlossaryEntries() {
		data.addTerm(entry)
	}
	user := data.addUser("stats@example.com", "Password!123", "user")
	data.addCase(user.ID, "chongqing", "错读", "chong2 qing4", "", "approved")
	data.addCase(user.ID, "yinhang", "错读", "yin2 hang2", "", "pending")
	server := newTestServer(t, data, testConfig(), testGlossaryEntries()...)

	recorder := performRequest(t, server.Handler(), http.MethodGet, "/api/stats", "")
	var stats store.Stats
	decodeJSON(t, recorder, &stats)
	if recorder.Code != http.StatusOK || stats.Terms != 2 || stats.Cases != 1 || stats.Pending != 1 || stats.Contributors != 1 {
		t.Fatalf("stats = %d %#v", recorder.Code, stats)
	}

	data.statsErr = errors.New("stats failed")
	if recorder := performRequest(t, server.Handler(), http.MethodGet, "/api/stats", ""); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("stats error status = %d", recorder.Code)
	}
}

func TestCreateCaseValidationAndSuccess(t *testing.T) {
	data := newFakeStore()
	for _, entry := range testGlossaryEntries() {
		data.addTerm(entry)
	}
	user := data.addUser("writer@example.com", "Password!123", "user")
	cookie := data.addSession(user)
	server := newTestServer(t, data, testConfig(), testGlossaryEntries()...)
	handler := server.Handler()

	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	recorder := performRequest(t, handler, http.MethodPost, "/api/cases", mustJSON(t, map[string]any{
		"term_id": "chongqing", "wrong_text": "重庆读错", "correct_pinyin": "chong2 qing4", "note": "测试",
	}), cookie)
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusCreated || response.ID == "" || response.Status != "pending" {
		t.Fatalf("create response = %d %#v", recorder.Code, response)
	}

	tests := []struct {
		name string
		body string
		code int
		err  string
	}{
		{name: "missing auth", body: `{}`, code: http.StatusUnauthorized, err: "unauthorized"},
		{name: "bad term", body: `{"term_id":"missing","wrong_text":"x","correct_pinyin":"y"}`, code: http.StatusBadRequest, err: "bad_term"},
		{name: "empty wrong", body: `{"term_id":"chongqing","wrong_text":" ","correct_pinyin":"y"}`, code: http.StatusBadRequest, err: "invalid_wrong_text"},
		{name: "long wrong", body: mustJSON(t, map[string]any{"term_id": "chongqing", "wrong_text": strings.Repeat("错", 501), "correct_pinyin": "y"}), code: http.StatusBadRequest, err: "invalid_wrong_text"},
		{name: "empty pinyin", body: `{"term_id":"chongqing","wrong_text":"x","correct_pinyin":""}`, code: http.StatusBadRequest, err: "invalid_pinyin"},
		{name: "long pinyin", body: mustJSON(t, map[string]any{"term_id": "chongqing", "wrong_text": "x", "correct_pinyin": strings.Repeat("a", 101)}), code: http.StatusBadRequest, err: "invalid_pinyin"},
		{name: "long note", body: mustJSON(t, map[string]any{"term_id": "chongqing", "wrong_text": "x", "correct_pinyin": "y", "note": strings.Repeat("注", 1001)}), code: http.StatusBadRequest, err: "invalid_note"},
		{name: "unknown field", body: `{"term_id":"chongqing","wrong_text":"x","correct_pinyin":"y","extra":1}`, code: http.StatusBadRequest, err: "bad_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cookies := []*http.Cookie{cookie}
			if test.name == "missing auth" {
				cookies = nil
			}
			recorder := performRequest(t, handler, http.MethodPost, "/api/cases", test.body, cookies...)
			var body errorResponse
			decodeJSON(t, recorder, &body)
			if recorder.Code != test.code || body.Code != test.err {
				t.Fatalf("status/code = %d %#v, want %d %s", recorder.Code, body, test.code, test.err)
			}
		})
	}

	data.createCaseErr = errors.New("insert failed")
	if recorder := performRequest(t, handler, http.MethodPost, "/api/cases", mustJSON(t, map[string]any{
		"term_id": "chongqing", "wrong_text": "x", "correct_pinyin": "y",
	}), cookie); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("create error status = %d", recorder.Code)
	}
}

func TestListCasesAndMyCases(t *testing.T) {
	data := newFakeStore()
	user := data.addUser("owner@example.com", "Password!123", "user")
	other := data.addUser("other@example.com", "Password!123", "user")
	cookie := data.addSession(user)
	approved := data.addCase(user.ID, "", "错读一", "dui4", "", "approved")
	pending := data.addCase(user.ID, "", "错读二", "dui4", "", "pending")
	data.addCase(other.ID, "", "别人的", "dui4", "", "approved")
	server := newTestServer(t, data, testConfig())
	handler := server.Handler()

	var publicResponse struct {
		Items []store.Case `json:"items"`
	}
	recorder := performRequest(t, handler, http.MethodGet, "/api/cases", "")
	decodeJSON(t, recorder, &publicResponse)
	if recorder.Code != http.StatusOK || len(publicResponse.Items) != 2 || publicResponse.Items[0].Status != "approved" {
		t.Fatalf("public cases = %d %#v", recorder.Code, publicResponse)
	}
	for _, item := range publicResponse.Items {
		if item.ID == pending.ID {
			t.Fatal("pending case leaked into public list")
		}
	}

	recorder = performRequest(t, handler, http.MethodGet, "/api/my/cases", "", cookie)
	decodeJSON(t, recorder, &publicResponse)
	if recorder.Code != http.StatusOK || len(publicResponse.Items) != 2 {
		t.Fatalf("my cases = %d %#v", recorder.Code, publicResponse)
	}
	foundApproved, foundPending := false, false
	for _, item := range publicResponse.Items {
		foundApproved = foundApproved || item.ID == approved.ID
		foundPending = foundPending || item.ID == pending.ID
	}
	if !foundApproved || !foundPending {
		t.Fatalf("my cases did not contain both records: %#v", publicResponse.Items)
	}

	if recorder := performRequest(t, handler, http.MethodGet, "/api/my/cases", ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated my cases = %d", recorder.Code)
	}

	data.listCasesErr = errors.New("list failed")
	if recorder := performRequest(t, handler, http.MethodGet, "/api/cases", ""); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("public cases error = %d", recorder.Code)
	}
}

func TestVoteAndUnvote(t *testing.T) {
	data := newFakeStore()
	user := data.addUser("voter@example.com", "Password!123", "user")
	cookie := data.addSession(user)
	approved := data.addCase(user.ID, "", "错读", "dui4", "", "approved")
	pending := data.addCase(user.ID, "", "待审", "dui4", "", "pending")
	server := newTestServer(t, data, testConfig())
	handler := server.Handler()

	var response struct {
		Votes int  `json:"votes"`
		Voted bool `json:"voted"`
	}
	recorder := performRequest(t, handler, http.MethodPost, "/api/cases/"+approved.ID+"/vote", "", cookie)
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.Votes != 1 || !response.Voted {
		t.Fatalf("vote = %d %#v", recorder.Code, response)
	}
	recorder = performRequest(t, handler, http.MethodPost, "/api/cases/"+approved.ID+"/vote", "", cookie)
	decodeJSON(t, recorder, &response)
	if response.Votes != 1 {
		t.Fatalf("duplicate vote should be idempotent: %#v", response)
	}

	recorder = performRequest(t, handler, http.MethodDelete, "/api/cases/"+approved.ID+"/vote", "", cookie)
	decodeJSON(t, recorder, &response)
	if recorder.Code != http.StatusOK || response.Votes != 0 || response.Voted {
		t.Fatalf("unvote = %d %#v", recorder.Code, response)
	}

	recorder = performRequest(t, handler, http.MethodPost, "/api/cases/"+pending.ID+"/vote", "", cookie)
	var body errorResponse
	decodeJSON(t, recorder, &body)
	if recorder.Code != http.StatusNotFound || body.Code != "not_found" {
		t.Fatalf("pending vote = %d %#v", recorder.Code, body)
	}

	recorder = performRequest(t, handler, http.MethodPost, "/api/cases/"+strings.Repeat("a", 65)+"/vote", "", cookie)
	decodeJSON(t, recorder, &body)
	if recorder.Code != http.StatusBadRequest || body.Code != "bad_id" {
		t.Fatalf("long id = %d %#v", recorder.Code, body)
	}
	if recorder := performRequest(t, handler, http.MethodPost, "/api/cases/"+approved.ID+"/vote", ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated vote = %d", recorder.Code)
	}

	data.voteErr = errors.New("vote failed")
	if recorder := performRequest(t, handler, http.MethodPost, "/api/cases/"+approved.ID+"/vote", "", cookie); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("vote error = %d", recorder.Code)
	}
	data.voteErr = nil
	data.unvoteErr = errors.New("unvote failed")
	if recorder := performRequest(t, handler, http.MethodDelete, "/api/cases/"+approved.ID+"/vote", "", cookie); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unvote error = %d", recorder.Code)
	}
}

func TestAdminCaseQueueAndModeration(t *testing.T) {
	data := newFakeStore()
	for _, entry := range testGlossaryEntries() {
		data.addTerm(entry)
	}
	user := data.addUser("normal@example.com", "Password!123", "user")
	admin := data.addUser("admin@example.com", "Password!123", "admin")
	userCookie := data.addSession(user)
	adminCookie := data.addSession(admin)
	item := data.addCase(user.ID, "chongqing", "错读", "chong2 qing4", "", "pending")
	server := newTestServer(t, data, testConfig(), testGlossaryEntries()...)
	handler := server.Handler()

	if recorder := performRequest(t, handler, http.MethodGet, "/api/admin/cases", "", userCookie); recorder.Code != http.StatusForbidden {
		t.Fatalf("normal user admin queue = %d", recorder.Code)
	}
	if recorder := performRequest(t, handler, http.MethodGet, "/api/admin/cases", ""); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous admin queue = %d", recorder.Code)
	}

	var listResponse struct {
		Items []store.Case `json:"items"`
	}
	recorder := performRequest(t, handler, http.MethodGet, "/api/admin/cases?status=pending", "", adminCookie)
	decodeJSON(t, recorder, &listResponse)
	if recorder.Code != http.StatusOK || len(listResponse.Items) != 1 || listResponse.Items[0].ID != item.ID {
		t.Fatalf("admin queue = %d %#v", recorder.Code, listResponse)
	}
	if recorder := performRequest(t, handler, http.MethodGet, "/api/admin/cases?status=invalid", "", adminCookie); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d", recorder.Code)
	}

	for _, test := range []struct {
		name string
		body string
		code int
		err  string
	}{
		{name: "invalid status", body: `{"status":"pending"}`, code: http.StatusBadRequest, err: "bad_status"},
		{name: "long note", body: mustJSON(t, map[string]any{"status": "approved", "note": strings.Repeat("注", 501)}), code: http.StatusBadRequest, err: "invalid_note"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := performRequest(t, handler, http.MethodPost, "/api/admin/cases/"+item.ID+"/moderate", test.body, adminCookie)
			var body errorResponse
			decodeJSON(t, recorder, &body)
			if recorder.Code != test.code || body.Code != test.err {
				t.Fatalf("moderate = %d %#v", recorder.Code, body)
			}
		})
	}

	recorder = performRequest(t, handler, http.MethodPost, "/api/admin/cases/"+item.ID+"/moderate", `{"status":"approved","note":"ok"}`, adminCookie)
	var moderation struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	decodeJSON(t, recorder, &moderation)
	if recorder.Code != http.StatusOK || !moderation.OK || moderation.Status != "approved" {
		t.Fatalf("moderate success = %d %#v", recorder.Code, moderation)
	}

	recorder = performRequest(t, handler, http.MethodPost, "/api/admin/cases/missing/moderate", `{"status":"approved"}`, adminCookie)
	var body errorResponse
	decodeJSON(t, recorder, &body)
	if recorder.Code != http.StatusNotFound || body.Code != "not_found" {
		t.Fatalf("missing moderation = %d %#v", recorder.Code, body)
	}

	data.moderateErr = errors.New("moderate failed")
	if recorder := performRequest(t, handler, http.MethodPost, "/api/admin/cases/"+item.ID+"/moderate", `{"status":"approved"}`, adminCookie); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("moderate error = %d", recorder.Code)
	}
	data.moderateErr = nil
	data.listCasesErr = errors.New("list failed")
	if recorder := performRequest(t, handler, http.MethodGet, "/api/admin/cases", "", adminCookie); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("admin list error = %d", recorder.Code)
	}
}
