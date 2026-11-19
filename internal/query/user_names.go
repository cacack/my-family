package query

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// UserNames resolves user IDs to display names (ADR-007). auth.Service
// implements it over the account store.
type UserNames interface {
	// DisplayNames returns the display name of each of ids that is a user.
	DisplayNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// UseUserNames lets the service name the user who made each change, in
// history entries and merge records. Without it they carry only the user ID.
func (s *HistoryService) UseUserNames(names UserNames) {
	s.userNames = names
}

// displayNames looks up the display name of each user ID in ids, skipping
// IDs that are not UUIDs. A failed lookup is logged and names no one: it
// annotates what a reader reports, it never decides whether it is shown.
func (s *HistoryService) displayNames(ctx context.Context, ids []string) map[string]string {
	if s == nil || s.userNames == nil || len(ids) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	parsed := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		parsed = append(parsed, id)
	}
	if len(parsed) == 0 {
		return nil
	}
	found, err := s.userNames.DisplayNames(ctx, parsed)
	if err != nil {
		slog.Warn("history: could not look up user display names", "error", err)
		return nil
	}
	names := make(map[string]string, len(found))
	for id, name := range found {
		names[id.String()] = name
	}
	return names
}

// nameUsers sets UserName on each entry whose UserID is a known user.
func (s *HistoryService) nameUsers(ctx context.Context, entries []ChangeEntry) {
	ids := make([]string, 0, len(entries))
	for i := range entries {
		if entries[i].UserID != nil {
			ids = append(ids, *entries[i].UserID)
		}
	}
	names := s.displayNames(ctx, ids)
	for i := range entries {
		if entries[i].UserID == nil {
			continue
		}
		if name, ok := names[*entries[i].UserID]; ok {
			entries[i].UserName = &name
		}
	}
}
