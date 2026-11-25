#!/bin/bash
# Record a release's event log for the replay test (#1802), into
# testdata/eventlogs/TAG/:
#
#   1. Check out TAG in a temporary worktree and run
#      testdata/eventlogs/record/main.go there, so the release's own code
#      fills an empty SQLite database with synthetic data (the demo seeder and
#      GEDCOM files from testdata/), reaching every event type its DecodeEvent
#      handles, and rebuilds it. It writes expected.json (the read-model values
#      the release's replay built) and event-types.json.
#   2. Write the database's log with this checkout's `myfamily backup`, as
#      backup.zip.
#
# Run it from a checkout of main right after tagging each minor release:
#
#   scripts/record-event-log.sh v0.17.0
#
# then commit the new directory. TestRecordedEventLogs_* in
# internal/integration replays every recorded log.
set -euo pipefail

if [ $# -ne 1 ]; then
	echo "usage: $0 TAG" >&2
	exit 2
fi
tag=$1

cd "$(dirname "$0")/.."
root=$(pwd)
out="$root/testdata/eventlogs/$tag"
if [ -e "$out" ]; then
	echo "$out already exists; a recorded log is never re-recorded" >&2
	exit 1
fi

work=$(mktemp -d)
cleanup() {
	git worktree remove --force "$work/src" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT

git worktree add --detach "$work/src" "$tag" >/dev/null
mkdir -p "$work/src/cmd/record-event-log" "$work/out"
cp testdata/eventlogs/record/main.go "$work/src/cmd/record-event-log/main.go"

gedcoms=(
	testdata/gedcom-5.5/comprehensive.ged
	testdata/gedcom-7.0/comprehensive.ged
	# Again, so two attributes exist: one to update, one to delete.
	testdata/gedcom-7.0/comprehensive.ged
	testdata/map-test.ged
	testdata/gedcom-5.5/555SAMPLE.GED
	testdata/gedcom-5.5/ancestry-sample.ged
	testdata/gedcom-5.5/name-sources-notes.ged
)
(cd "$work/src" && go run ./cmd/record-event-log "$work/recorded.db" "$work/out" "${gedcoms[@]/#/$root/}")

# env -u: DEMO_MODE or DATABASE_URL in the caller's environment would back up
# another database.
env -u DEMO_MODE -u DATABASE_URL SQLITE_PATH="$work/recorded.db" \
	go run ./cmd/myfamily backup "$work/out/backup.zip"

mkdir -p "$out"
cp "$work/out/backup.zip" "$work/out/expected.json" "$work/out/event-types.json" "$out/"
echo "Recorded $tag in $out"
