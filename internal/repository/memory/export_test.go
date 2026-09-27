package memory

import (
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// StoredMediaBytes exposes, to this package's external tests only, the bytes
// physically stored on the (branch, id) media row and whether that row exists
// (tombstones included), so the #759 scenario can prove a branch shadow row
// stores no copy of main's bytes.
func StoredMediaBytes(s *ReadModelStore, branch domain.BranchID, id uuid.UUID) (file, thumb []byte, present bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.media[branchKey{branch, id}]
	if !ok {
		return nil, nil, false
	}
	return r.m.FileData, r.m.ThumbnailData, true
}
