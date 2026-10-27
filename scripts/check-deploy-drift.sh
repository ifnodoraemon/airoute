#!/bin/sh
# Read-only drift check for the long-lived dependency containers.
#
# Why this exists: docker-compose only applies `ulimits` and `command` when a
# container is CREATED. Rebuilding the gateway image does not touch the
# postgres/redis/lb containers, so an old deployment can keep running with the
# 1024-descriptor default and PostgreSQL's old max_connections=100 while the
# compose file claims 65536/200. The result looked like random "too many open
# files" and "remaining connection slots are reserved" failures after an
# otherwise successful upgrade.
#
# This script never mutates anything. It reports drift and prints the exact
# recreate command. Run it before/after a rollout (see docs/RELEASE_GUIDE.md).
set -u

want_nofile=65536
want_pg_max_connections=200

failed=0
warn() { printf 'WARN: %s\n' "$*" >&2; failed=1; }
info() { printf 'INFO: %s\n' "$*"; }

if ! command -v docker >/dev/null 2>&1; then
    info "docker not found; skipping dependency drift check"
    exit 0
fi

container_running() {
    [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null || true)" = "true" ]
}

container_nofile() {
    docker exec "$1" sh -c 'ulimit -n' 2>/dev/null | tr -d '[:space:]'
}

check_nofile() {
    name=$1
    if ! container_running "$name"; then
        info "$name is not running; skipped"
        return
    fi
    got=$(container_nofile "$name")
    case "$got" in
        ''|*[!0-9]*)
            info "$name: could not read RLIMIT_NOFILE (no shell in image?)"
            ;;
        *)
            if [ "$got" -lt "$want_nofile" ]; then
                warn "$name nofile=$got < $want_nofile (created before the compose ulimit); recreate it: docker compose up -d --force-recreate $name"
            else
                info "$name nofile=$got OK"
            fi
            ;;
    esac
}

for name in airoute-postgres airoute-redis airoute-1 airoute-2 airoute-lb; do
    check_nofile "$name"
done

if container_running airoute-postgres; then
    pg_user=$(docker exec airoute-postgres sh -c 'printf %s "$POSTGRES_USER"' 2>/dev/null | tr -d '[:space:]')
    [ -n "$pg_user" ] || pg_user=airoute
    pg_max=$(docker exec airoute-postgres psql -U "$pg_user" -d postgres -tAc 'show max_connections' 2>/dev/null | tr -d '[:space:]')
    case "$pg_max" in
        ''|*[!0-9]*)
            info "airoute-postgres: could not read max_connections"
            ;;
        *)
            if [ "$pg_max" -lt "$want_pg_max_connections" ]; then
                warn "airoute-postgres max_connections=$pg_max < $want_pg_max_connections (old server config); recreate it: docker compose up -d --force-recreate airoute-postgres"
            else
                info "airoute-postgres max_connections=$pg_max OK"
            fi
            ;;
    esac
fi

if [ "$failed" -eq 0 ]; then
    info "no dependency container drift detected"
fi
exit "$failed"
