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
# Records which checkout owns each stack so `gc` can find stacks left by removed worktrees.
registry="${XDG_STATE_HOME:-$HOME/.local/state}/subcult-os/dev-env/projects"

dc() {
  docker compose --project-directory "$root" --env-file /dev/null \
    -p "$project" -f "$root/compose.dev.yml" "$@"
}

toolchain() {
  mise exec node@24.18.0 go@1.26.11 pnpm@10.33.0 -- "$@"
}

setup() {
  command -v mise >/dev/null
  command -v docker >/dev/null
  # A no-op `mise install` still repairs shared runtime symlinks, which can need
  # sudo on hosts with a system tool directory; skip it when the pins are present.
  mise install --dry-run-code node@24.18.0 go@1.26.11 pnpm@10.33.0 >/dev/null 2>&1 ||
    mise install node@24.18.0 go@1.26.11 pnpm@10.33.0
  toolchain make deps
  dc config --quiet
}

url() {
  local binding
  binding=$(dc port web 5173)
  [[ -n "$binding" ]] || { echo 'Run Start Dev first.' >&2; return 1; }
  printf 'http://%s\n' "$binding"
}

acquire_test_lock() {
  # Cooperate with the installed T3 launcher and other repository worktrees.
  local state="$HOME/.local/state/t3-dev-environments/fleet"
  mkdir -p "$state"
  exec 8>"$state/host-test.lock"
  flock 8
}

cleanup_test_db() {
  local result=$?
  dc logs --no-color test-db > .cache/dev-env/test-db.log 2>&1 ||
    echo 'Could not retain test database logs.' >&2
  if ! dc rm -sf test-db >/dev/null; then
    echo 'Could not remove the disposable test database container.' >&2
    if [[ "$result" == 0 ]]; then result=1; fi
  fi
  # An idle project network still holds one of the host's Docker address pools.
  if [[ -z "$(dc ps -aq 2>/dev/null)" ]]; then
    dc down >/dev/null 2>&1 || echo 'Could not release the idle dev network.' >&2
  fi
  return "$result"
}

test_db() {
  # Serializes this worktree's DB tests; other worktrees remain independent.
  mkdir -p .cache/dev-env
  exec 9>.cache/dev-env/test-db.lock
  flock 9
  trap cleanup_test_db EXIT
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
    mkdir -p "$registry"
    printf '%s\n' "$root" > "$registry/$project"
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
  test) acquire_test_lock; toolchain make test ;;
  seed) toolchain node scripts/dev-seed.mjs "$(url)" "$(dc ps -q postgres)" "$project" ;;
  test-db) acquire_test_lock; test_db ;;
  verify)
    acquire_test_lock
    toolchain make verify
    test_db
    ;;
  url) url ;;
  status) dc ps ;;
  logs) dc logs --tail 100 -f api web ;;
  # Named volumes keep the data; the containers and network are recreated by start.
  stop) dc down ;;
  # Deletes this worktree's stack including its development database.
  down)
    dc --profile test down -v --remove-orphans
    rm -f "$registry/$project"
    ;;
  # Reports (or with --apply removes) dev stacks whose checkout no longer exists.
  gc) shift; python3 "$root/scripts/dev-stack-gc.py" --repo "$root" --registry "$registry" "$@" ;;
  *)
    echo 'Usage: bash scripts/dev-env.sh {setup|start|watch|seed|test|test-db|verify|url|status|logs|stop|down|gc}'
    [[ "${1:-help}" == help ]]
    ;;
esac
