package repository

import (
	"context"
	"errors"
	"fmt"
)

// ReadModelSchemaVersion is the read-model schema this binary projects into
// (#680, ADR-006). The read model is derived from the event log, so a schema
// change is applied by `myfamily rebuild`, which re-creates the read-model
// tables and replays the log, not by migrating rows in place.
//
// Bump it for any read-model change that an additive ALTER TABLE ADD COLUMN at
// startup cannot apply: a primary key, constraint or column type, a dropped
// column, or a change to what an existing event projects. `serve` then refuses
// a database stamped with another version until it is rebuilt.
//
// Versions:
//   - 0: no stamp. Every database from before #680, whatever its schema.
//   - 1: the first stamped version (#680).
const ReadModelSchemaVersion = 1

// ErrReadModelSchemaMismatch is returned by CheckReadModelSchema when the
// database's read model is not at ReadModelSchemaVersion.
var ErrReadModelSchemaMismatch = errors.New("the read model's schema version does not match this version of myfamily")

// CheckReadModelSchema fails, wrapping ErrReadModelSchemaMismatch, unless the
// read model is stamped with ReadModelSchemaVersion (#680). It never rebuilds:
// a rebuild needs every server stopped, so the error tells the operator to run
// `myfamily rebuild`, which stamps the version when it re-creates the tables.
//
// A database with no stamp and an empty event log is new: its tables were just
// created at this version, so it is stamped and passes. One with no stamp but
// with events predates the stamp, and its schema is unknown.
func CheckReadModelSchema(ctx context.Context, events EventStore, rs ReadModelStore) error {
	version, err := rs.GetSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("read the read model's schema version: %w", err)
	}
	if version == ReadModelSchemaVersion {
		return nil
	}
	if version == 0 {
		head, err := events.ReadAll(ctx, 0, 1)
		if err != nil {
			return fmt.Errorf("read the event log: %w", err)
		}
		if len(head) == 0 {
			if err := rs.SetSchemaVersion(ctx, ReadModelSchemaVersion); err != nil {
				return fmt.Errorf("stamp the read model's schema version: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: the database is at version %d and this binary needs version %d; "+
		"stop every server and run `myfamily rebuild` to rebuild the read model from the event log",
		ErrReadModelSchemaMismatch, version, ReadModelSchemaVersion)
}
