#!/bin/sh
# Rebuild the gateway image with the version derived from the checkout and roll
# both replicas onto it one at a time, so the load balancer keeps serving while
# the cluster is replaced. Mirrors what the CI release workflow publishes.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"

# VERSION can be pinned by the caller (for example a release tag); otherwise it
# is derived from the checkout, exactly like the CI release workflow does.
version=${VERSION:-$(sh scripts/build-version.sh)}
echo "==> Building airoute image ($version)..."
VERSION="$version" docker compose build

# Reconcile the dependency containers' own compose config (postgres
# max_connections, redis/LB nofile limits, ...). `up -d` only recreates a
# container when its config actually changed, so steady-state rebuilds stay a
# no-op. A dependency recreate can briefly interrupt requests; that is the price
# of applying a changed capacity setting, so keep such edits deliberate.
echo "==> Reconciling dependency containers (postgres, redis, storage)..."
VERSION="$version" docker compose up -d --no-deps postgres redis storage
for container in airoute-postgres airoute-redis airoute-storage; do
    status=
    for _ in $(seq 1 30); do
        status=$(docker inspect -f '{{.State.Health.Status}}' "$container" 2>/dev/null || printf missing)
        [ "$status" = healthy ] && break
        sleep 2
    done
    if [ "$status" != healthy ]; then
        echo "$container did not become healthy (last status: ${status:-unknown})" >&2
        exit 1
    fi
done

# Recreate replicas back to front: the remaining node keeps answering through
# the load balancer while the other one starts and passes its health check.
for service in gateway-2 gateway-1; do
    container="airoute-${service#gateway-}"
    echo "==> Recreating $service..."
    VERSION="$version" docker compose up -d --no-deps --force-recreate "$service"

    status=
    for _ in $(seq 1 30); do
        status=$(docker inspect -f '{{.State.Health.Status}}' "$container" 2>/dev/null || printf missing)
        [ "$status" = healthy ] && break
        sleep 2
    done
    if [ "$status" != healthy ]; then
        echo "$container did not become healthy (last status: ${status:-unknown})" >&2
        docker logs --tail 20 "$container" >&2 || true
        exit 1
    fi
    echo "==> $container is healthy"
done

# The edge container carries its own ulimits and resolves upstream addresses at
# (re)load time. Recreate it only when its config changed, then force a reload so
# a freshly recreated gateway is never routed through a stale upstream IP.
echo "==> Reconciling load balancer..."
VERSION="$version" docker compose up -d --no-deps lb
docker exec airoute-lb nginx -s reload || true

echo "==> Gateway cluster is running version $version"
