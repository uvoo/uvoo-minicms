#!/usr/bin/env bash
set -euo pipefail

image="${CMS_IMAGE:-uvoo-minicms:ci}"
container="${CMS_CONTAINER:-uvoo-minicms-smoke}"
port="${CMS_SMOKE_PORT:-18080}"
password="${CMS_ADMIN_PASS:-ci-smoke-password}"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
}

cleanup
trap cleanup EXIT

docker build -t "$image" .
docker run -d \
  --name "$container" \
  -p "127.0.0.1:${port}:8080" \
  -e "CMS_ADMIN_PASS=${password}" \
  "$image" >/dev/null

for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${port}/" >/dev/null 2>&1; then
    exit 0
  fi
  sleep 1
done

echo "container did not become ready on http://127.0.0.1:${port}/" >&2
docker logs "$container" >&2 || true
exit 1
