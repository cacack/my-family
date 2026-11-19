package repository

import "testing"

func TestRole_Includes(t *testing.T) {
	roles := []Role{RoleViewer, RoleEditor, RoleOwner}
	for i, r := range roles {
		for j, other := range roles {
			if got, want := r.Includes(other), i >= j; got != want {
				t.Errorf("%s.Includes(%s) = %v, want %v", r, other, got, want)
			}
		}
	}
	for _, r := range []Role{"", "admin", "Owner"} {
		if r.Valid() {
			t.Errorf("Role(%q).Valid() = true, want false", r)
		}
		if r.Includes(RoleViewer) {
			t.Errorf("Role(%q).Includes(viewer) = true, want false", r)
		}
	}
}
