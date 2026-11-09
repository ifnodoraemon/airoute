#!/usr/bin/env bash
set -euo pipefail
repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
go_binary=${GO_BINARY:-go}
if ! command -v "$go_binary" >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  go_binary=/usr/local/go/bin/go
fi
pg_container=
cleanup() {
  if [ -n "$pg_container" ]; then docker rm -f "$pg_container" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT
# Disposable data, credentials and database. No deployment volumes or DSNs.
pg_container=$(docker run --rm -d --label airoute.test=notifications \
  --tmpfs /var/lib/postgresql/data:rw -p 127.0.0.1::5432 \
  -e POSTGRES_USER=notification_test -e POSTGRES_PASSWORD=synthetic-test-only \
  -e POSTGRES_DB=notification_test postgres:16-alpine)
ready=false
for attempt in $(seq 1 40); do
  if docker exec "$pg_container" pg_isready -h 127.0.0.1 -U notification_test -d notification_test >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 0.25
done
if [ "$ready" != true ]; then
  printf '%s\n' 'Disposable PostgreSQL did not become ready.' >&2
  exit 1
fi
pg_binding=$(docker port "$pg_container" 5432/tcp)
pg_port=${pg_binding##*:}
cd -- "$repo_dir"
AIROUTE_TEST_DATABASE_URL="postgres://notification_test:synthetic-test-only@127.0.0.1:$pg_port/notification_test?sslmode=disable" \
  "$go_binary" test -race -v -tags=integration ./internal/storage -run '^TestPostgresNotifications$' -count=1
