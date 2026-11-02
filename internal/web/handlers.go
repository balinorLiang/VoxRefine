package web

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"polyphone-checker/internal/glossary"
	"polyphone-checker/internal/store"
)

type checkRequest struct {
	Text       string `json:"text"`
	MaxMatches int    `json:"max_matches"`
}

type createCaseRequest struct {
	TermID        string `json:"term_id"`
	WrongText     string `json:"wrong_text"`
	CorrectPinyin string `json:"correct_pinyin"`
	Note          string `json:"note"`
}

type moderateRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

var termTypes = map[string]bool{
	"place": true, "surname": true, "word": true, "name": true, "idiom": true, "brand": true,
}

var risks = map[string]bool{"high": true, "medium": true}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	if !s.checkLimiter.allow("check:" + s.clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "检查请求过于频繁，请稍后再试", "rate_limited")
		return
	}
	var request checkRequest
	if err := readJSON(w, r, &request); err != nil {
		apiError(w, http.StatusBadRequest, "请求格式不正确", "bad_request")
		return
	}
	if strings.TrimSpace(request.Text) == "" {
		apiError(w, http.StatusBadRequest, "请先输入要检查的文案", "empty_text")
		return
	}
	if len(request.Text) > 200_000 {
		apiError(w, http.StatusRequestEntityTooLarge, "单次检查最多 200,000 字节，请分段检查", "text_too_long")
		return
	}
	matches, truncated := s.detector.Find(request.Text, request.MaxMatches)
	for i := range matches {
		matches[i].Start = utf8.RuneCountInString(request.Text[:matches[i].Start])
		matches[i].End = utf8.RuneCountInString(request.Text[:matches[i].End])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"matches":     matches,
		"total":       len(matches),
		"truncated":   truncated,
		"text_length": utf8.RuneCountInString(request.Text),
		"engine":      "glossary-v2",
	})
}

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(query) > 100 {
		apiError(w, http.StatusBadRequest, "搜索词太长", "query_too_long")
		return
	}
	termType := strings.TrimSpace(r.URL.Query().Get("type"))
	risk := strings.TrimSpace(r.URL.Query().Get("risk"))
	if termType != "" && !termTypes[termType] {
		apiError(w, http.StatusBadRequest, "未知的词条类型", "bad_type")
		return
	}
	if risk != "" && !risks[risk] {
		apiError(w, http.StatusBadRequest, "未知的风险等级", "bad_risk")
		return
	}
	limit := queryInt(r, "limit", 50, 1, 100)
	offset := queryInt(r, "offset", 0, 0, 10_000)
	items, total, err := s.store.SearchTerms(r.Context(), query, termType, risk, limit, offset)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "词库读取失败", "internal_error")
		return
	}
	if items == nil {
		items = []glossary.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"query":  query,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleTerm(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 100 {
		apiError(w, http.StatusBadRequest, "词条 ID 不正确", "bad_id")
		return
	}
	entry, err := s.store.GetTerm(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		apiError(w, http.StatusNotFound, "没有找到这个词条", "not_found")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "词库读取失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"term": entry})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, "统计读取失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleCases(w http.ResponseWriter, r *http.Request) {
	viewerID := ""
	if user, ok := s.currentUser(r); ok {
		viewerID = user.ID
	}
	limit := queryInt(r, "limit", 50, 1, 100)
	offset := queryInt(r, "offset", 0, 0, 10_000)
	items, err := s.store.ListCases(r.Context(), "approved", viewerID, "", limit, offset)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "案例读取失败", "internal_error")
		return
	}
	if items == nil {
		items = []store.Case{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleMyCases(w http.ResponseWriter, r *http.Request, user store.User) {
	limit := queryInt(r, "limit", 50, 1, 100)
	offset := queryInt(r, "offset", 0, 0, 10_000)
	items, err := s.store.ListCases(r.Context(), "", user.ID, user.ID, limit, offset)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "案例读取失败", "internal_error")
		return
	}
	if items == nil {
		items = []store.Case{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateCase(w http.ResponseWriter, r *http.Request, user store.User) {
	if !s.writeLimiter.allow("case:" + s.clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "提交过于频繁，请稍后再试", "rate_limited")
		return
	}
	var request createCaseRequest
	if err := readJSON(w, r, &request); err != nil {
		apiError(w, http.StatusBadRequest, "请求格式不正确", "bad_request")
		return
	}
	request.TermID = strings.TrimSpace(request.TermID)
	request.WrongText = strings.TrimSpace(request.WrongText)
	request.CorrectPinyin = strings.TrimSpace(request.CorrectPinyin)
	request.Note = strings.TrimSpace(request.Note)

	if request.TermID != "" {
		if _, err := s.store.GetTerm(r.Context(), request.TermID); errors.Is(err, store.ErrNotFound) {
			apiError(w, http.StatusBadRequest, "关联的词条不存在", "bad_term")
			return
		} else if err != nil {
			apiError(w, http.StatusInternalServerError, "词条读取失败", "internal_error")
			return
		}
	}
	if utf8.RuneCountInString(request.WrongText) < 1 || utf8.RuneCountInString(request.WrongText) > 500 {
		apiError(w, http.StatusBadRequest, "错误文本需要在 1 到 500 个字之间", "invalid_wrong_text")
		return
	}
	if utf8.RuneCountInString(request.CorrectPinyin) < 1 || utf8.RuneCountInString(request.CorrectPinyin) > 100 {
		apiError(w, http.StatusBadRequest, "正确读音需要在 1 到 100 个字之间", "invalid_pinyin")
		return
	}
	if utf8.RuneCountInString(request.Note) > 1000 {
		apiError(w, http.StatusBadRequest, "补充说明最多 1000 个字", "invalid_note")
		return
	}
	id, err := s.store.CreateCase(r.Context(), user.ID, request.TermID, request.WrongText, request.CorrectPinyin, request.Note)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "案例提交失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending"})
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request, user store.User) {
	if !s.writeLimiter.allow("vote:" + s.clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "操作过于频繁，请稍后再试", "rate_limited")
		return
	}
	caseID := strings.TrimSpace(r.PathValue("id"))
	if caseID == "" || len(caseID) > 64 {
		apiError(w, http.StatusBadRequest, "案例 ID 不正确", "bad_id")
		return
	}
	votes, err := s.store.VoteCase(r.Context(), caseID, user.ID)
	if errors.Is(err, store.ErrNotFound) {
		apiError(w, http.StatusNotFound, "案例不存在或尚未通过审核", "not_found")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "投票失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"votes": votes, "voted": true})
}

func (s *Server) handleUnvote(w http.ResponseWriter, r *http.Request, user store.User) {
	caseID := strings.TrimSpace(r.PathValue("id"))
	if caseID == "" || len(caseID) > 64 {
		apiError(w, http.StatusBadRequest, "案例 ID 不正确", "bad_id")
		return
	}
	votes, err := s.store.UnvoteCase(r.Context(), caseID, user.ID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "取消投票失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"votes": votes, "voted": false})
}

func (s *Server) handleAdminCases(w http.ResponseWriter, r *http.Request, user store.User) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "pending"
	}
	if status != "pending" && status != "approved" && status != "rejected" && status != "all" {
		apiError(w, http.StatusBadRequest, "未知的审核状态", "bad_status")
		return
	}
	if status == "all" {
		status = ""
	}
	limit := queryInt(r, "limit", 50, 1, 100)
	offset := queryInt(r, "offset", 0, 0, 10_000)
	items, err := s.store.ListCases(r.Context(), status, "", "", limit, offset)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "审核列表读取失败", "internal_error")
		return
	}
	if items == nil {
		items = []store.Case{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleModerate(w http.ResponseWriter, r *http.Request, user store.User) {
	if !s.writeLimiter.allow("moderate:" + s.clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		apiError(w, http.StatusTooManyRequests, "操作过于频繁，请稍后再试", "rate_limited")
		return
	}
	caseID := strings.TrimSpace(r.PathValue("id"))
	if caseID == "" || len(caseID) > 64 {
		apiError(w, http.StatusBadRequest, "案例 ID 不正确", "bad_id")
		return
	}
	var request moderateRequest
	if err := readJSON(w, r, &request); err != nil {
		apiError(w, http.StatusBadRequest, "请求格式不正确", "bad_request")
		return
	}
	request.Status = strings.TrimSpace(request.Status)
	request.Note = strings.TrimSpace(request.Note)
	if request.Status != "approved" && request.Status != "rejected" {
		apiError(w, http.StatusBadRequest, "审核状态只能是 approved 或 rejected", "bad_status")
		return
	}
	if utf8.RuneCountInString(request.Note) > 500 {
		apiError(w, http.StatusBadRequest, "审核备注最多 500 个字", "invalid_note")
		return
	}
	if err := s.store.ModerateCase(r.Context(), caseID, user.ID, request.Status, request.Note); errors.Is(err, store.ErrNotFound) {
		apiError(w, http.StatusNotFound, "案例不存在", "not_found")
		return
	} else if err != nil {
		apiError(w, http.StatusInternalServerError, "审核失败", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": request.Status})
}
