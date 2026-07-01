#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_PORT="${BACKEND_PORT:-3000}"
FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"

backend_pid=""
frontend_pid=""

require_command() {
  local command_name="$1"
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "Missing command: ${command_name}" >&2
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
  trap - INT TERM EXIT

  if [[ -n "${frontend_pid}" ]] && kill -0 "${frontend_pid}" >/dev/null 2>&1; then
    kill "${frontend_pid}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${backend_pid}" ]] && kill -0 "${backend_pid}" >/dev/null 2>&1; then
    kill "${backend_pid}" >/dev/null 2>&1 || true
  fi

  wait "${frontend_pid}" "${backend_pid}" >/dev/null 2>&1 || true
}

require_command go
require_command npm

if [[ ! -d "${ROOT_DIR}/web/node_modules" && ! -d "${ROOT_DIR}/web/default/node_modules" ]]; then
  echo "Frontend dependencies are missing. Install them before running this script." >&2
  exit 1
fi

# main.go embeds both frontend build folders at compile time. Dev uses the
# Rsbuild server for the real UI, so ignored placeholder files are enough here.
ensure_embed_index "${ROOT_DIR}/web/default/dist/index.html"
ensure_embed_index "${ROOT_DIR}/web/classic/dist/index.html"

trap cleanup INT TERM EXIT

echo "Starting backend:  http://localhost:${BACKEND_PORT}"
(
  cd "${ROOT_DIR}"
  PORT="${BACKEND_PORT}" go run main.go
) &
backend_pid="$!"

echo "Starting default frontend: http://localhost:${FRONTEND_PORT}"
(
  cd "${ROOT_DIR}/web/default"
  npm run dev -- --host "${FRONTEND_HOST}" --port "${FRONTEND_PORT}"
) &
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
