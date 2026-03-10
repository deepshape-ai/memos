package v1

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

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

// dailyLogDayBounds returns the [start, end) unix timestamps for the day
// that the request targets. The client MUST provide a createTime representing
// the start-of-day in the user's local timezone (e.g. dayjs().startOf("day")).
// We use that timestamp directly instead of truncating to UTC midnight,
// because users in non-UTC timezones would get the wrong calendar day.
// If no timestamp is provided we fall back to the server's current UTC day.
func dailyLogDayBounds(memo *v1pb.Memo) (int64, int64) {
	var anchor time.Time
	switch {
	case memo.GetCreateTime() != nil && memo.GetCreateTime().IsValid():
		anchor = memo.GetCreateTime().AsTime()
	case memo.GetDisplayTime() != nil && memo.GetDisplayTime().IsValid():
		anchor = memo.GetDisplayTime().AsTime()
	default:
		anchor = time.Now().UTC().Truncate(24 * time.Hour)
	}
	// The client sends midnight-in-their-timezone as the anchor.
	// Truncate to the second to remove sub-second precision, then use as-is.
	dayStart := anchor.Truncate(time.Second)
	return dayStart.Unix(), dayStart.Add(24 * time.Hour).Unix()
}

// isTodayDailyLog checks whether a stored daily log memo was created within
// the last 36 hours. The wider window (vs 24h) accommodates the full range
// of UTC offsets (UTC-12 to UTC+14) so that the backend never locks a user
// out of editing "today's" log due to server/client timezone mismatch.
// The frontend enforces the exact calendar-day boundary via its own isToday
// check; the backend acts as a lenient safety net.
func isTodayDailyLog(memo *store.Memo) bool {
	created := time.Unix(memo.CreatedTs, 0)
	return time.Since(created) < 36*time.Hour
}

func isSupportedDailyLogUpdatePath(path string) bool {
	return path == "content" || path == "update_time"
}

func (s *APIV1Service) findDailyLogMemoForDay(ctx context.Context, creatorID int32, memo *v1pb.Memo) (*store.Memo, error) {
	startTs, endTs := dailyLogDayBounds(memo)
	limit := 1
	rowStatus := store.Normal
	filter := fmt.Sprintf(`memo_type == "DAILY_LOG" && created_ts >= %d && created_ts < %d`, startTs, endTs)
	existingMemo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		CreatorID:       &creatorID,
		RowStatus:       &rowStatus,
		ExcludeComments: true,
		Filters:         []string{filter},
		Limit:           &limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to find daily log")
	}
	return existingMemo, nil
}

func (s *APIV1Service) saveDailyLogMemo(ctx context.Context, user *store.User, request *v1pb.CreateMemoRequest, create *store.Memo) (*store.Memo, bool, error) {
	existingMemo, err := s.findDailyLogMemoForDay(ctx, user.ID, request.Memo)
	if err != nil {
		return nil, false, err
	}

	return s.upsertDailyLogMemo(ctx, existingMemo, create, func() int64 {
		if request.Memo.UpdateTime != nil && request.Memo.UpdateTime.IsValid() {
			return request.Memo.UpdateTime.AsTime().Unix()
		}
		return time.Now().Unix()
	})
}

// saveDailyLogForDate is a simplified upsert used by the REST handler.
// It finds an existing log for the given day range and either creates or
// updates accordingly.
func (s *APIV1Service) saveDailyLogForDate(ctx context.Context, user *store.User, create *store.Memo, dayStart time.Time) (*store.Memo, bool, error) {
	startTs := dayStart.Unix()
	endTs := dayStart.Add(24 * time.Hour).Unix()
	limit := 1
	rowStatus := store.Normal
	filter := fmt.Sprintf(`memo_type == "DAILY_LOG" && created_ts >= %d && created_ts < %d`, startTs, endTs)
	existingMemo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		CreatorID:       &user.ID,
		RowStatus:       &rowStatus,
		ExcludeComments: true,
		Filters:         []string{filter},
		Limit:           &limit,
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to find daily log")
	}

	return s.upsertDailyLogMemo(ctx, existingMemo, create, func() int64 {
		return time.Now().Unix()
	})
}

// upsertDailyLogMemo is the shared upsert logic for daily logs.
// If existingMemo is nil, a new memo is created. Otherwise, only today's
// log can be updated (past logs are immutable).
func (s *APIV1Service) upsertDailyLogMemo(ctx context.Context, existingMemo *store.Memo, create *store.Memo, updatedTsFn func() int64) (*store.Memo, bool, error) {
	if existingMemo == nil {
		memo, err := s.Store.CreateMemo(ctx, create)
		if err != nil {
			return nil, false, err
		}
		return memo, true, nil
	}

	// Only today's daily log can be updated. Past logs are immutable.
	if !isTodayDailyLog(existingMemo) {
		return nil, false, errors.New("past daily logs cannot be modified; use +/- prefixes in today's log instead")
	}

	updatedTs := updatedTsFn()
	// Daily logs are always visible to the workspace (Protected).
	visibility := store.Protected
	update := &store.UpdateMemo{
		ID:         existingMemo.ID,
		Content:    &create.Content,
		Payload:    create.Payload,
		Visibility: &visibility,
		UpdatedTs:  &updatedTs,
	}
	if err := s.Store.UpdateMemo(ctx, update); err != nil {
		return nil, false, errors.Wrap(err, "failed to update existing daily log")
	}

	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{ID: &existingMemo.ID})
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to reload existing daily log")
	}
	return memo, false, nil
}
