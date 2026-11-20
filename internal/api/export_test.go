package api

// UnprotectVisitorsForTest has s refuse visitors, as if
// repository.AnonymousViewingProtected were false, so the kill switch can be
// tested while it is off.
func (s *Server) UnprotectVisitorsForTest() {
	s.visitorsProtected = false
}
