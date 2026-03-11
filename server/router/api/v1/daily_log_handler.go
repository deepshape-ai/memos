package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/lithammer/shortuuid/v4"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/server/runner/memopayload"
	"github.com/usememos/memos/store"
)

// -----------------------------------------------------------------------
// Response types
//
// Designed for AI agents and automation:
//   - snake_case fields matching proto Memo names where possible.
//   - `date` is always YYYY-MM-DD (UTC-based from created_ts).
//   - `editable` tells callers whether the log can still be modified.
//   - Timestamps are RFC 3339 / UTC.
// -----------------------------------------------------------------------

// dailyLogResponse is a single daily log entry.
type dailyLogResponse struct {
	// Resource name: "memos/{uid}".
	Name string `json:"name"`
	// Creator resource name: "users/{id}".
	Creator string `json:"creator"`
	// Plain-text content with .plan-style line prefixes.
	Content string `json:"content"`
	// Calendar date this log belongs to (YYYY-MM-DD, UTC).
	Date string `json:"date"`
	// Whether the authenticated user can still edit this log.
	Editable bool `json:"editable"`
	// RFC 3339 UTC timestamps.
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
}

// listDailyLogsResponse wraps a paginated list of daily logs.
type listDailyLogsResponse struct {
	DailyLogs     []dailyLogResponse `json:"daily_logs"`
	NextPageToken string             `json:"next_page_token,omitempty"`
	TotalSize     int                `json:"total_size"`
}

// apiError is a structured JSON error body.
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func makeDailyLogResponse(memo *store.Memo, editable bool) dailyLogResponse {
	return dailyLogResponse{
		Name:       fmt.Sprintf("%s%s", MemoNamePrefix, memo.UID),
		Creator:    fmt.Sprintf("%s%d", UserNamePrefix, memo.CreatorID),
		Content:    memo.Content,
		Date:       time.Unix(memo.CreatedTs, 0).UTC().Format("2006-01-02"),
		Editable:   editable,
		CreateTime: time.Unix(memo.CreatedTs, 0).UTC().Format(time.RFC3339),
		UpdateTime: time.Unix(memo.UpdatedTs, 0).UTC().Format(time.RFC3339),
	}
}

func jsonError(c *echo.Context, code int, msg string) error {
	return c.JSON(code, apiError{Code: code, Message: msg})
}

func parseDatePathParam(dateStr string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
	}
	return t, nil
}

func parseCreatorQueryParam(raw string) (*int32, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := ExtractUserIDFromName(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid creator: expected format users/{id}")
	}
	return &id, nil
}

func clampPageSize(raw string, defaultSize int) int {
	if raw == "" {
		return defaultSize
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return defaultSize
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return defaultSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return n
}

func buildDailyLogFilter(startTs, endTs *int64) string {
	parts := []string{`memo_type == "DAILY_LOG"`}
	if startTs != nil {
		parts = append(parts, fmt.Sprintf("created_ts >= %d", *startTs))
	}
	if endTs != nil {
		parts = append(parts, fmt.Sprintf("created_ts < %d", *endTs))
	}
	return strings.Join(parts, " && ")
}

// -----------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------

// RegisterDailyLogRoutes registers the Daily Log REST API.
//
// Endpoints:
//
//	PUT    /api/v1/daily-logs/:date           Create or update today's daily log
//	GET    /api/v1/daily-logs/:date           Get a daily log by date
//	DELETE /api/v1/daily-logs/:date           Delete a daily log (admin only)
//	GET    /api/v1/daily-logs                 List daily logs (with filtering & pagination)
func (s *APIV1Service) RegisterDailyLogRoutes(g *echo.Group) {
	authenticator := auth.NewAuthenticator(s.Store, s.Secret)

	authMW := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			result := authenticator.Authenticate(c.Request().Context(), authHeader)
			if result == nil {
				return jsonError(c, http.StatusUnauthorized, "authentication required")
			}
			ctx := auth.ApplyToContext(c.Request().Context(), result)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}

	g.PUT("/api/v1/daily-logs/:date", s.handleSaveDailyLog, authMW)
	g.GET("/api/v1/daily-logs/:date", s.handleGetDailyLog, authMW)
	g.DELETE("/api/v1/daily-logs/:date", s.handleDeleteDailyLog, authMW)
	g.GET("/api/v1/daily-logs", s.handleListDailyLogs, authMW)
}

// -----------------------------------------------------------------------
// PUT /api/v1/daily-logs/:date
//
// Creates or updates the authenticated user's daily log for the given date.
// Only today's log (within a 36-hour backend window) can be saved.
//
// Request body:
//
//	{ "content": "* finished auth module\n+ TODO: write tests" }
//
// Returns: dailyLogResponse (201 Created or 200 OK).
// -----------------------------------------------------------------------

func (s *APIV1Service) handleSaveDailyLog(c *echo.Context) error {
	ctx := c.Request().Context()

	user, err := s.fetchCurrentUser(ctx)
	if err != nil || user == nil {
		return jsonError(c, http.StatusUnauthorized, "user not found")
	}

	dateStr := c.Param("date")
	dayStart, err := parseDatePathParam(dateStr)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, err.Error())
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body: expected JSON with \"content\" field")
	}

	content := normalizeDailyLogContent(body.Content)
	if content == "" {
		return jsonError(c, http.StatusBadRequest, "content must not be empty")
	}
	if err := validateDailyLogContent(content); err != nil {
		return jsonError(c, http.StatusBadRequest, err.Error())
	}

	contentLengthLimit, err := s.getContentLengthLimit(ctx)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "failed to check content length limit")
	}
	if len(content) > contentLengthLimit {
		return jsonError(c, http.StatusBadRequest, fmt.Sprintf("content too long (max %d characters)", contentLengthLimit))
	}

	memoUID := shortuuid.New()

	createdTs := dayStart.Unix()
	create := &store.Memo{
		UID:        memoUID,
		CreatorID:  user.ID,
		Content:    content,
		Visibility: store.Protected,
		CreatedTs:  createdTs,
		UpdatedTs:  createdTs,
		Payload: &storepb.MemoPayload{
			Type: storepb.MemoPayload_DAILY_LOG,
		},
	}
	if err := memopayload.RebuildMemoPayload(create, s.MarkdownService); err != nil {
		return jsonError(c, http.StatusInternalServerError, "failed to rebuild payload")
	}

	memo, isNew, err := s.saveDailyLogForDate(ctx, user, create, dayStart)
	if err != nil {
		if strings.Contains(err.Error(), "past daily logs") {
			return jsonError(c, http.StatusForbidden, err.Error())
		}
		return jsonError(c, http.StatusInternalServerError, "failed to save daily log")
	}

	// Broadcast SSE event for live UI refresh.
	memoName := fmt.Sprintf("%s%s", MemoNamePrefix, memo.UID)
	if isNew {
		s.SSEHub.Broadcast(&SSEEvent{Type: SSEEventMemoCreated, Name: memoName})
	} else {
		s.SSEHub.Broadcast(&SSEEvent{Type: SSEEventMemoUpdated, Name: memoName})
	}

	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	return c.JSON(status, makeDailyLogResponse(memo, true))
}

// -----------------------------------------------------------------------
// GET /api/v1/daily-logs/:date
//
// Returns the daily log for the given date.
//
// Query params:
//   - creator: "users/{id}" — defaults to current user.
//
// Returns: dailyLogResponse (200) or 404.
// -----------------------------------------------------------------------

func (s *APIV1Service) handleGetDailyLog(c *echo.Context) error {
	ctx := c.Request().Context()

	user, err := s.fetchCurrentUser(ctx)
	if err != nil || user == nil {
		return jsonError(c, http.StatusUnauthorized, "user not found")
	}

	dateStr := c.Param("date")
	dayStart, err := parseDatePathParam(dateStr)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, err.Error())
	}

	creatorID := user.ID
	if raw := c.QueryParam("creator"); raw != "" {
		id, err := ExtractUserIDFromName(raw)
		if err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid creator: expected format users/{id}")
		}
		creatorID = id
	}

	startTs := dayStart.Unix()
	endTs := dayStart.Add(24 * time.Hour).Unix()
	limit := 1
	rowStatus := store.Normal
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		CreatorID:       &creatorID,
		RowStatus:       &rowStatus,
		ExcludeComments: true,
		Filters:         []string{buildDailyLogFilter(&startTs, &endTs)},
		Limit:           &limit,
	})
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "failed to query daily log")
	}
	if memo == nil {
		return jsonError(c, http.StatusNotFound, fmt.Sprintf("no daily log found for %s", dateStr))
	}

	editable := memo.CreatorID == user.ID && isTodayDailyLog(memo)
	return c.JSON(http.StatusOK, makeDailyLogResponse(memo, editable))
}

// -----------------------------------------------------------------------
// DELETE /api/v1/daily-logs/:date
//
// Deletes a daily log. Admin-only; regular users cannot delete daily logs.
//
// Query params:
//   - creator: "users/{id}" — defaults to current user.
//
// Returns: 204 No Content.
// -----------------------------------------------------------------------

func (s *APIV1Service) handleDeleteDailyLog(c *echo.Context) error {
	ctx := c.Request().Context()

	user, err := s.fetchCurrentUser(ctx)
	if err != nil || user == nil {
		return jsonError(c, http.StatusUnauthorized, "user not found")
	}
	if !isSuperUser(user) {
		return jsonError(c, http.StatusForbidden, "only admins can delete daily logs")
	}

	dateStr := c.Param("date")
	dayStart, err := parseDatePathParam(dateStr)
	if err != nil {
		return jsonError(c, http.StatusBadRequest, err.Error())
	}

	creatorID := user.ID
	if raw := c.QueryParam("creator"); raw != "" {
		id, err := ExtractUserIDFromName(raw)
		if err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid creator: expected format users/{id}")
		}
		creatorID = id
	}

	startTs := dayStart.Unix()
	endTs := dayStart.Add(24 * time.Hour).Unix()
	limit := 1
	rowStatus := store.Normal
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		CreatorID:       &creatorID,
		RowStatus:       &rowStatus,
		ExcludeComments: true,
		Filters:         []string{buildDailyLogFilter(&startTs, &endTs)},
		Limit:           &limit,
	})
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "failed to query daily log")
	}
	if memo == nil {
		return jsonError(c, http.StatusNotFound, fmt.Sprintf("no daily log found for %s", dateStr))
	}

	if err := s.Store.DeleteMemo(ctx, &store.DeleteMemo{ID: memo.ID}); err != nil {
		return jsonError(c, http.StatusInternalServerError, "failed to delete daily log")
	}

	memoName := fmt.Sprintf("%s%s", MemoNamePrefix, memo.UID)
	s.SSEHub.Broadcast(&SSEEvent{Type: SSEEventMemoDeleted, Name: memoName})

	return c.NoContent(http.StatusNoContent)
}

// -----------------------------------------------------------------------
// GET /api/v1/daily-logs
//
// Lists daily logs with optional date range, creator, and pagination.
//
// Query params:
//   - start_date:  Inclusive start (YYYY-MM-DD).
//   - end_date:    Exclusive end (YYYY-MM-DD).
//   - creator:     "users/{id}" — empty means all visible logs.
//   - page_size:   1-1000, default 50.
//   - page_token:  Opaque token from a previous response.
//
// Returns: listDailyLogsResponse (200).
// -----------------------------------------------------------------------

func (s *APIV1Service) handleListDailyLogs(c *echo.Context) error {
	ctx := c.Request().Context()

	user, err := s.fetchCurrentUser(ctx)
	if err != nil || user == nil {
		return jsonError(c, http.StatusUnauthorized, "user not found")
	}

	// Parse date range.
	var startTs, endTs *int64
	if raw := c.QueryParam("start_date"); raw != "" {
		t, err := parseDatePathParam(raw)
		if err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid start_date: "+err.Error())
		}
		ts := t.Unix()
		startTs = &ts
	}
	if raw := c.QueryParam("end_date"); raw != "" {
		t, err := parseDatePathParam(raw)
		if err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid end_date: "+err.Error())
		}
		ts := t.Unix()
		endTs = &ts
	}

	// Parse optional creator.
	creatorID, err := parseCreatorQueryParam(c.QueryParam("creator"))
	if err != nil {
		return jsonError(c, http.StatusBadRequest, err.Error())
	}

	// Build find query.
	find := &store.FindMemo{
		CreatorID:       creatorID,
		ExcludeComments: true,
		Filters:         []string{buildDailyLogFilter(startTs, endTs)},
	}
	rowStatus := store.Normal
	find.RowStatus = &rowStatus

	// Visibility: own logs always visible; others' logs need PROTECTED/PUBLIC.
	if creatorID == nil {
		visibilityFilter := fmt.Sprintf(`creator_id == %d || visibility in ["PUBLIC", "PROTECTED"]`, user.ID)
		find.Filters = append(find.Filters, visibilityFilter)
	} else if *creatorID != user.ID {
		find.VisibilityList = []store.Visibility{store.Public, store.Protected}
	}

	// Pagination (reuses the same proto-based page token as ListMemos).
	pageSize := clampPageSize(c.QueryParam("page_size"), 50)
	var offset int
	if pt := c.QueryParam("page_token"); pt != "" {
		var pageToken v1pb.PageToken
		if err := unmarshalPageToken(pt, &pageToken); err != nil {
			return jsonError(c, http.StatusBadRequest, "invalid page_token")
		}
		offset = int(pageToken.Offset)
	}

	limitPlusOne := pageSize + 1
	find.Limit = &limitPlusOne
	find.Offset = &offset

	memos, err := s.Store.ListMemos(ctx, find)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "failed to list daily logs")
	}

	// Determine next page token.
	var nextPageToken string
	if len(memos) == limitPlusOne {
		memos = memos[:pageSize]
		nextPageToken, _ = getPageToken(pageSize, offset+pageSize)
	}

	results := make([]dailyLogResponse, 0, len(memos))
	for _, m := range memos {
		editable := m.CreatorID == user.ID && isTodayDailyLog(m)
		results = append(results, makeDailyLogResponse(m, editable))
	}

	return c.JSON(http.StatusOK, listDailyLogsResponse{
		DailyLogs:     results,
		NextPageToken: nextPageToken,
		TotalSize:     len(results),
	})
}
