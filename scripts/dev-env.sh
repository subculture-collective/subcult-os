#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$root"
# Identity always comes from this checkout, never from an inherited deployment env.
digest=$(printf '%s' "$root" | sha256sum)
project="subcult-dev-${digest:0:16}"
export DEV_UID DEV_GID
DEV_UID=$(id -u)
DEV_GID=$(id -g)
unset DEV_WEB_URL

dc() {
  docker compose --project-directory "$root" --env-file /dev/null \
    -p "$project" -f "$root/compose.dev.yml" "$@"
}

toolchain() {
  mise exec node@24.18.0 go@1.26.6 pnpm@10.33.0 -- "$@"
}

setup() {
  command -v mise >/dev/null
  command -v docker >/dev/null
  mise install node@24.18.0 go@1.26.6 pnpm@10.33.0
  toolchain make deps
  dc config --quiet
}

url() {
  local binding
  binding=$(dc port web 5173)
  [[ -n "$binding" ]] || { echo 'Run Start Dev first.' >&2; return 1; }
  printf 'http://%s\n' "$binding"
}

test_db() {
  # Serializes this worktree's DB tests; other worktrees remain independent.
  mkdir -p .cache/dev-env
  exec 9>.cache/dev-env/test-db.lock
  flock 9
  trap 'dc rm -sf test-db >/dev/null' EXIT
  dc up -d --wait --wait-timeout 90 test-db
  local binding
  binding=$(dc port test-db 5432)
  TEST_DATABASE_URL="postgres://test:disposable-test-only@${binding}/test?sslmode=disable" \
    toolchain make test-db
}

case "${1:-help}" in
  setup) setup ;;
  start)
    setup
    dc up -d --wait --wait-timeout 120 postgres web
    DEV_WEB_URL=$(url)
    export DEV_WEB_URL
    dc up -d --wait --wait-timeout 240 api
    printf '\nDev environment: %s\nPreview: %s\n' "$project" "$DEV_WEB_URL"
    curl --fail --silent --show-error "$DEV_WEB_URL/api/health"
    printf '\n'
    ;;
  watch)
    DEV_WEB_URL=$(url)
    export DEV_WEB_URL
    dc watch --no-up
    ;;
  test) toolchain make test ;;
  seed) toolchain node scripts/dev-seed.mjs "$(url)" "$(dc ps -q postgres)" "$project" ;;
  test-db) test_db ;;
  verify)
    toolchain make verify
    test_db
    ;;
  url) url ;;
  status) dc ps ;;
  logs) dc logs --tail 100 -f api web ;;
  stop) dc stop ;;
  *)
    echo 'Usage: bash scripts/dev-env.sh {setup|start|watch|seed|test|test-db|verify|url|status|logs|stop}'
    [[ "${1:-help}" == help ]]
    ;;
esac
