package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lithammer/shortuuid/v4"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/pkg/errors"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

// dailyLogJSON is the canonical response shape for MCP daily-log results.
type dailyLogJSON struct {
	Name          string `json:"name"`
	Creator       string `json:"creator"`
	Content       string `json:"content"`
	Date          string `json:"date"`
	Editable      bool   `json:"editable"`
	CreateTime    int64  `json:"create_time"`
	UpdateTime    int64  `json:"update_time"`
	CreateTimeISO string `json:"create_time_iso"`
	UpdateTimeISO string `json:"update_time_iso"`
}

func storeMemoToDailyLogJSON(m *store.Memo, callerID int32) dailyLogJSON {
	return dailyLogJSON{
		Name:          "memos/" + m.UID,
		Creator:       fmt.Sprintf("users/%d", m.CreatorID),
		Content:       m.Content,
		Date:          time.Unix(m.CreatedTs, 0).UTC().Format("2006-01-02"),
		Editable:      m.CreatorID == callerID && isDailyLogToday(m),
		CreateTime:    m.CreatedTs,
		UpdateTime:    m.UpdatedTs,
		CreateTimeISO: time.Unix(m.CreatedTs, 0).UTC().Format(time.RFC3339),
		UpdateTimeISO: time.Unix(m.UpdatedTs, 0).UTC().Format(time.RFC3339),
	}
}

// ---------------------------------------------------------------------------
// Daily-log content helpers (duplicated from api/v1 to avoid cross-package
// dependency; these are pure functions with no external state).
// ---------------------------------------------------------------------------

func normalizeDailyLogContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.TrimSpace(content)
}

func validateDailyLogContent(content string) error {
	normalized := normalizeDailyLogContent(content)
	for index, line := range strings.Split(normalized, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return errors.Errorf("daily log line %d must be flat", index+1)
		}
		if strings.HasPrefix(strings.TrimSpace(line), "====") {
			return errors.Errorf("daily log line %d must not contain day separators", index+1)
		}
		if len(line) > 0 && strings.ContainsRune("*+-?", rune(line[0])) && !strings.HasPrefix(line, fmt.Sprintf("%c ", line[0])) {
			return errors.Errorf("daily log line %d has an invalid prefix", index+1)
		}
	}
	return nil
}

// isDailyLogToday checks whether a stored daily log was created within the
// last 36 hours (same window as the REST API).
func isDailyLogToday(memo *store.Memo) bool {
	created := time.Unix(memo.CreatedTs, 0)
	return time.Since(created) < 36*time.Hour
}

func dailyLogFilter(startTs, endTs *int64) string {
	parts := []string{`memo_type == "DAILY_LOG"`}
	if startTs != nil {
		parts = append(parts, fmt.Sprintf("created_ts >= %d", *startTs))
	}
	if endTs != nil {
		parts = append(parts, fmt.Sprintf("created_ts < %d", *endTs))
	}
	return strings.Join(parts, " && ")
}

// ---------------------------------------------------------------------------
// Tool registration
// ---------------------------------------------------------------------------

func (s *MCPService) registerDailyLogTools(mcpSrv *mcpserver.MCPServer) {
	mcpSrv.AddTool(mcp.NewTool("memos_save_daily_log",
		mcp.WithDescription("Create or update today's daily log. Content must use .plan-style line prefixes: "+
			`"* " (done), "+ " (to-do), "- " (note), "? " (question). `+
			"Each date has exactly one log per user. Only today's log can be saved; past logs are immutable. "+
			"Visibility is always PROTECTED. Requires authentication."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("date", mcp.Required(), mcp.Description("Date for the log entry (YYYY-MM-DD). Must be today's date.")),
		mcp.WithString("content", mcp.Required(), mcp.Description(`Daily log content with .plan-style prefixes, e.g. "* finished auth\n+ TODO: write tests"`)),
	), s.handleSaveDailyLog)

	mcpSrv.AddTool(mcp.NewTool("memos_get_daily_log",
		mcp.WithDescription("Get a daily log by date. Defaults to the caller's log. Requires authentication."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("date", mcp.Required(), mcp.Description("Date to retrieve (YYYY-MM-DD)")),
		mcp.WithString("creator", mcp.Description(`Optional creator filter, e.g. "users/1". Defaults to the authenticated user.`)),
	), s.handleGetDailyLog)

	mcpSrv.AddTool(mcp.NewTool("memos_list_daily_logs",
		mcp.WithDescription("List daily logs with optional date range and creator filter. "+
			"Visibility rules: own logs are always visible; others' logs require PROTECTED or PUBLIC visibility. "+
			"Requires authentication."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("start_date", mcp.Description("Inclusive start date (YYYY-MM-DD)")),
		mcp.WithString("end_date", mcp.Description("Exclusive end date (YYYY-MM-DD)")),
		mcp.WithString("creator", mcp.Description(`Optional creator filter, e.g. "users/1"`)),
		mcp.WithNumber("page_size", mcp.Description("Maximum logs to return (1–100, default 20)")),
		mcp.WithNumber("page", mcp.Description("Zero-based page index for pagination (default 0)")),
	), s.handleListDailyLogs)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *MCPService) handleSaveDailyLog(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := extractUserID(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error() + " Create a Personal Access Token in Settings > My Account > Access Tokens."), nil
	}

	dateStr := req.GetString("date", "")
	if dateStr == "" {
		return mcp.NewToolResultError("date is required (YYYY-MM-DD). Provide today's date."), nil
	}
	dayStart, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return mcp.NewToolResultError("invalid date format, expected YYYY-MM-DD (e.g., 2024-01-15)."), nil
	}

	content := normalizeDailyLogContent(req.GetString("content", ""))
	if content == "" {
		return mcp.NewToolResultError("content must not be empty. Use .plan-style prefixes: * (done), + (to-do), - (note), ? (question)."), nil
	}
	if err := validateDailyLogContent(content); err != nil {
		return mcp.NewToolResultError(err.Error() + " Each line must start with * + - or ? followed by a space. No indentation allowed."), nil
	}

	// Find existing log for the day.
	startTs := dayStart.Unix()
	endTs := dayStart.Add(24 * time.Hour).Unix()
	limit := 1
	rowStatus := store.Normal
	existing, err := s.store.GetMemo(ctx, &store.FindMemo{
		CreatorID:       &userID,
		RowStatus:       &rowStatus,
		ExcludeComments: true,
		Filters:         []string{dailyLogFilter(&startTs, &endTs)},
		Limit:           &limit,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to find daily log: %v", err)), nil
	}

	var memo *store.Memo
	var isNew bool

	if existing == nil {
		// Create new daily log.
		memo, err = s.store.CreateMemo(ctx, &store.Memo{
			UID:        shortuuid.New(),
			CreatorID:  userID,
			Content:    content,
			Visibility: store.Protected,
			CreatedTs:  dayStart.Unix(),
			UpdatedTs:  dayStart.Unix(),
			Payload: &storepb.MemoPayload{
				Type: storepb.MemoPayload_DAILY_LOG,
			},
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create daily log: %v", err)), nil
		}
		isNew = true
	} else {
		// Only today's log can be updated.
		if !isDailyLogToday(existing) {
			return mcp.NewToolResultError("past daily logs cannot be modified; use +/- prefixes in today's log instead. Daily logs are immutable after the 36-hour window."), nil
		}
		visibility := store.Protected
		updatedTs := time.Now().Unix()
		update := &store.UpdateMemo{
			ID:         existing.ID,
			Content:    &content,
			Visibility: &visibility,
			UpdatedTs:  &updatedTs,
			Payload: &storepb.MemoPayload{
				Type: storepb.MemoPayload_DAILY_LOG,
			},
		}
		if err := s.store.UpdateMemo(ctx, update); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to update daily log: %v", err)), nil
		}
		memo, err = s.store.GetMemo(ctx, &store.FindMemo{ID: &existing.ID})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to reload daily log: %v", err)), nil
		}
		isNew = false
	}

	_ = isNew // not needed in MCP response

	out, err := marshalJSON(storeMemoToDailyLogJSON(memo, userID))
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(out), nil
}

func (s *MCPService) handleGetDailyLog(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := extractUserID(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error() + " Create a Personal Access Token in Settings > My Account > Access Tokens."), nil
	}

	dateStr := req.GetString("date", "")
	if dateStr == "" {
		return mcp.NewToolResultError("date is required (YYYY-MM-DD). Provide a date like 2024-01-15."), nil
	}
	dayStart, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return mcp.NewToolResultError("invalid date format, expected YYYY-MM-DD (e.g., 2024-01-15)."), nil
	}

	creatorID := userID
	if raw := req.GetString("creator", ""); raw != "" {
		id, parseErr := parseCreatorName(raw)
		if parseErr != nil {
			return mcp.NewToolResultError(parseErr.Error() + ` Use format "users/<id>".`), nil
		}
		creatorID = id
	}

	startTs := dayStart.Unix()
	endTs := dayStart.Add(24 * time.Hour).Unix()
	limit := 1
	rowStatus := store.Normal
	memo, err := s.store.GetMemo(ctx, &store.FindMemo{
		CreatorID:       &creatorID,
		RowStatus:       &rowStatus,
		ExcludeComments: true,
		Filters:         []string{dailyLogFilter(&startTs, &endTs)},
		Limit:           &limit,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to query daily log: %v", err)), nil
	}
	if memo == nil {
		return mcp.NewToolResultError(fmt.Sprintf("no daily log found for %s. Use memos_list_daily_logs to see available logs.", dateStr)), nil
	}

	// Access control: own logs always visible; others' need PROTECTED/PUBLIC.
	if memo.CreatorID != userID {
		if err := checkMemoAccess(memo, userID); err != nil {
			return mcp.NewToolResultError(err.Error() + " Ensure you have permission to view this log."), nil
		}
	}

	out, err := marshalJSON(storeMemoToDailyLogJSON(memo, userID))
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(out), nil
}

func (s *MCPService) handleListDailyLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID, err := extractUserID(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error() + " Create a Personal Access Token in Settings > My Account > Access Tokens."), nil
	}

	// Parse date range.
	var startTs, endTs *int64
	if raw := req.GetString("start_date", ""); raw != "" {
		t, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			return mcp.NewToolResultError("invalid start_date, expected YYYY-MM-DD (e.g., 2024-01-15)."), nil
		}
		ts := t.Unix()
		startTs = &ts
	}
	if raw := req.GetString("end_date", ""); raw != "" {
		t, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			return mcp.NewToolResultError("invalid end_date, expected YYYY-MM-DD (e.g., 2024-01-15)."), nil
		}
		ts := t.Unix()
		endTs = &ts
	}

	// Parse optional creator.
	var creatorID *int32
	if raw := req.GetString("creator", ""); raw != "" {
		id, parseErr := parseCreatorName(raw)
		if parseErr != nil {
			return mcp.NewToolResultError(parseErr.Error() + ` Use format "users/<id>".`), nil
		}
		creatorID = &id
	}

	// Pagination.
	pageSize := req.GetInt("page_size", 20)
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	page := req.GetInt("page", 0)
	if page < 0 {
		page = 0
	}

	limit := pageSize + 1
	offset := page * pageSize
	rowStatus := store.Normal
	find := &store.FindMemo{
		CreatorID:       creatorID,
		RowStatus:       &rowStatus,
		ExcludeComments: true,
		Filters:         []string{dailyLogFilter(startTs, endTs)},
		Limit:           &limit,
		Offset:          &offset,
	}

	// Visibility: own logs always visible; others' need PROTECTED/PUBLIC.
	if creatorID == nil {
		find.Filters = append(find.Filters, fmt.Sprintf(`creator_id == %d || visibility in ["PUBLIC", "PROTECTED"]`, userID))
	} else if *creatorID != userID {
		find.VisibilityList = []store.Visibility{store.Public, store.Protected}
	}

	memos, err := s.store.ListMemos(ctx, find)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list daily logs: %v", err)), nil
	}

	hasMore := len(memos) > pageSize
	if hasMore {
		memos = memos[:pageSize]
	}

	results := make([]dailyLogJSON, len(memos))
	for i, m := range memos {
		results[i] = storeMemoToDailyLogJSON(m, userID)
	}

	type listResponse struct {
		DailyLogs []dailyLogJSON `json:"daily_logs"`
		HasMore   bool           `json:"has_more"`
		Page      int            `json:"page"`
		PageSize  int            `json:"page_size"`
	}
	out, err := marshalJSON(listResponse{DailyLogs: results, HasMore: hasMore, Page: page, PageSize: pageSize})
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(out), nil
}

// parseCreatorName extracts a user ID from "users/{id}" format.
func parseCreatorName(name string) (int32, error) {
	prefix := "users/"
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok || rest == "" {
		return 0, errors.Errorf(`creator must be in the format "users/{id}", got %q`, name)
	}
	var id int32
	for _, ch := range rest {
		if ch < '0' || ch > '9' {
			return 0, errors.Errorf(`creator must be in the format "users/{id}", got %q`, name)
		}
		id = id*10 + int32(ch-'0')
	}
	if id <= 0 {
		return 0, errors.Errorf(`creator must be in the format "users/{id}", got %q`, name)
	}
	return id, nil
}
