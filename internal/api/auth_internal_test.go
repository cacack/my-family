package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cacack/my-family/internal/auth"
)

// TestThrottled pins the errors every hashing endpoint maps to 429.
func TestThrottled(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{auth.ErrRateLimited, true},
		{auth.ErrBusy, true},
		{fmt.Errorf("sign in: %w", auth.ErrBusy), true},
		{auth.ErrInvalidCredentials, false},
		{errors.New("boom"), false},
		{nil, false},
	} {
		if got := throttled(tc.err); got != tc.want {
			t.Errorf("throttled(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
