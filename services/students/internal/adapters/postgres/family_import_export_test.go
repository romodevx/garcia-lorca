package postgres

// SetMaxUndoLinksForTest lowers the undo's read ceiling so a test can watch a
// batch undone over several calls; the returned func restores it.
func SetMaxUndoLinksForTest(n int) (restore func()) {
	saved := maxUndoLinks
	maxUndoLinks = n
	return func() { maxUndoLinks = saved }
}
