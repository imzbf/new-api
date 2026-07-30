#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_PORT="${BACKEND_PORT:-3000}"
FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"

backend_pid=""
frontend_pid=""

terminate_process_tree() {
  local root_pid="$1"
  local child_pid

  if [[ -z "${root_pid}" ]]; then
    return
  fi

  # go run and Bun scripts both spawn child processes that may own the real
  # listening socket, so cleanup must walk descendants instead of killing only
  # the wrapper process captured by $!.
  while IFS= read -r child_pid; do
    [[ -n "${child_pid}" ]] || continue
    terminate_process_tree "${child_pid}"
  done < <(pgrep -P "${root_pid}" 2>/dev/null || true)

  if kill -0 "${root_pid}" >/dev/null 2>&1; then
    kill "${root_pid}" >/dev/null 2>&1 || true
  fi
}

cleanup_managed_child() {
  trap - INT TERM

  if [[ -n "${child_pid:-}" ]]; then
    terminate_process_tree "${child_pid}"
    wait "${child_pid}" >/dev/null 2>&1 || true
  fi
}

start_managed_process() {
  local workdir="$1"
  shift
  local child_pid=""
  local child_status

  trap cleanup_managed_child INT TERM

  cd "${workdir}"
  "$@" &
  child_pid="$!"

  set +e
  wait "${child_pid}"
  child_status="$?"
  set -e

  cleanup_managed_child
  return "${child_status}"
}

require_command() {
  local command_name="$1"
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "Missing command: ${command_name}" >&2
    exit 1
  fi
}

assert_port_available() {
  local port="$1"
  local label="$2"
  local env_name="$3"

  if lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "${label} port ${port} is already in use." >&2
    echo "Stop the process below, or set ${env_name} to another free port:" >&2
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN >&2 || true
    exit 1
  fi
}

ensure_embed_index() {
  local file_path="$1"
  if [[ -f "${file_path}" ]]; then
    return
  fi

  mkdir -p "$(dirname "${file_path}")"
  cat >"${file_path}" <<'HTML'
<!doctype html>
<html>
  <head>
    <title>dev</title>
  </head>
  <body>
    use frontend dev server
  </body>
</html>
HTML
}

cleanup() {
  local exit_code=$?

  trap - INT TERM EXIT

  terminate_process_tree "${frontend_pid}"
  terminate_process_tree "${backend_pid}"

  for pid in "${frontend_pid}" "${backend_pid}"; do
    [[ -n "${pid}" ]] || continue
    wait "${pid}" >/dev/null 2>&1 || true
  done

  exit "${exit_code}"
}

require_command go
require_command bun
require_command pgrep
require_command lsof

if [[ ! -d "${ROOT_DIR}/web/node_modules" ]]; then
  echo "Frontend dependencies are missing. Run 'bun install' in web/ before starting development." >&2
  exit 1
fi

if [[ "${BACKEND_PORT}" == "${FRONTEND_PORT}" ]]; then
  echo "Backend and frontend cannot use the same port: ${BACKEND_PORT}" >&2
  exit 1
fi

# Fail before starting either side so a stale backend/frontend cannot leave the
# other service half-running or emit a misleading SIGTERM lifecycle error.
assert_port_available "${BACKEND_PORT}" "Backend" "BACKEND_PORT"
assert_port_available "${FRONTEND_PORT}" "Frontend" "FRONTEND_PORT"

# main.go embeds web/dist at compile time. Dev uses the Rsbuild server for the
# real UI, so an ignored placeholder file is enough when no production build exists.
ensure_embed_index "${ROOT_DIR}/web/dist/index.html"

trap cleanup INT TERM EXIT

echo "Starting backend:  http://localhost:${BACKEND_PORT}"
start_managed_process "${ROOT_DIR}" env PORT="${BACKEND_PORT}" go run main.go &
backend_pid="$!"

echo "Starting frontend: http://localhost:${FRONTEND_PORT}"
start_managed_process "${ROOT_DIR}/web" bun run dev -- --host "${FRONTEND_HOST}" --port "${FRONTEND_PORT}" &
frontend_pid="$!"

echo
echo "Development environment is starting."
echo "Press Ctrl+C to stop backend and frontend."

while true; do
  if ! kill -0 "${backend_pid}" >/dev/null 2>&1; then
    wait "${backend_pid}"
    exit "$?"
  fi

  if ! kill -0 "${frontend_pid}" >/dev/null 2>&1; then
    wait "${frontend_pid}"
    exit "$?"
  fi

  sleep 1
done
