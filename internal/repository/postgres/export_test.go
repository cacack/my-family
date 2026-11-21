package postgres

// MediaBlobLockKey exposes the media advisory lock key to the external tests.
const MediaBlobLockKey = mediaBlobLockKey

// ReadModelTables exposes the tables ResetReadModel empties to the external tests.
var ReadModelTables = readModelTables

// CleanupDeletes exposes the session and audit cleanup deletes, by table, so
// a test can pin that an index serves each (#1433).
var CleanupDeletes = map[string]string{
	"sessions":   deleteExpiredSessionsSQL,
	"auth_audit": deleteOldAuthAuditSQL,
}
