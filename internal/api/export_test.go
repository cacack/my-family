package api

// ProtectVisitorsForTest lets s serve visitors whose tree's anonymous access
// is viewer, as if repository.AnonymousViewingProtected were true, so the
// visitor paths can be tested before the read side protects them.
func (s *Server) ProtectVisitorsForTest() {
	s.visitorsProtected = true
}
