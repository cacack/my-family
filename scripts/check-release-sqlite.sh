#!/bin/bash
# Prove that a binary built the way releases are built (.goreleaser.yaml:
# CGO_ENABLED=0) runs on SQLite, the default storage (#822).
#
#   1. Build ./cmd/myfamily with CGO_ENABLED=0 and start `serve` on a fresh
#      SQLite file: create a person, find them through search, restart the
#      server and find them again.
#   2. Cross-compile every GOOS/GOARCH pair .goreleaser.yaml ships, so a
#      platform the pure-Go SQLite driver does not support fails here rather
#      than at release time.
#
# internal/web/dist must hold something for go:embed (CI stages a placeholder).
set -euo pipefail

cd "$(dirname "$0")/.."

work=$(mktemp -d)
server_pid=""
cleanup() {
	if [ -n "$server_pid" ]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -rf "$work"
}
trap cleanup EXIT

echo "=== Building a cgo-less binary (as .goreleaser.yaml does) ==="
CGO_ENABLED=0 go build -o "$work/myfamily" ./cmd/myfamily

port=${CHECK_PORT:-18422}
base="http://127.0.0.1:$port/api/v1"
db="$work/myfamily.db"

start_server() {
	if curl -fsS "$base/persons" >/dev/null 2>&1; then
		echo "::error::something is already listening on port $port; set CHECK_PORT to a free port"
		exit 1
	fi
	# env -u: DEMO_MODE or DATABASE_URL in the caller's environment would select
	# another backend, and the check would pass without touching SQLite.
	env -u DEMO_MODE -u DATABASE_URL SQLITE_PATH="$db" PORT="$port" \
		"$work/myfamily" serve >"$work/serve.log" 2>&1 &
	server_pid=$!
	for _ in $(seq 1 50); do
		if curl -fsS "$base/persons" >/dev/null 2>&1; then
			return 0
		fi
		if ! kill -0 "$server_pid" 2>/dev/null; then
			echo "::error::myfamily serve exited on SQLite; log follows"
			cat "$work/serve.log"
			exit 1
		fi
		sleep 0.2
	done
	echo "::error::myfamily serve did not come up on SQLite; log follows"
	cat "$work/serve.log"
	exit 1
}

stop_server() {
	kill "$server_pid"
	wait "$server_pid" 2>/dev/null || true
	server_pid=""
}

expect_found() {
	local query=$1 id=$2
	if ! curl -fsS "$base/search?q=$query" | grep -q "\"$id\""; then
		echo "::error::search for $query did not return person $id"
		exit 1
	fi
}

echo "=== Running serve on SQLite ==="
start_server
if ! grep -q "SQLite ($db)" "$work/serve.log"; then
	echo "::error::serve did not report the SQLite store in use; log follows"
	cat "$work/serve.log"
	exit 1
fi
id=$(curl -fsS -X POST "$base/persons" -H 'Content-Type: application/json' \
	-d '{"given_name":"Release","surname":"Smoketest"}' |
	sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
if [ -z "$id" ]; then
	echo "::error::creating a person on SQLite returned no id"
	exit 1
fi
expect_found Smoketest "$id"
stop_server

echo "=== Restarting on the same SQLite file ==="
start_server
expect_found Smoketest "$id"
stop_server
echo "OK: the cgo-less binary opens, writes and searches SQLite"

echo "=== Cross-compiling every release target ==="
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	echo "  $target"
	CGO_ENABLED=0 GOOS=${target%/*} GOARCH=${target#*/} go build -o /dev/null ./cmd/myfamily
done
echo "OK: every release target builds without cgo"
