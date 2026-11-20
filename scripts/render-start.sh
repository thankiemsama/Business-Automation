#!/usr/bin/env bash
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"
export APP_ORIGIN="${APP_ORIGIN:-${RENDER_EXTERNAL_URL:-}}"
export BACKEND_URL=http://127.0.0.1:8080
export HTTP_ADDR=127.0.0.1:8080

nexa-api &
api_pid=$!
web_pid=
cleanup() {
    kill "$api_pid" ${web_pid:+"$web_pid"} 2>/dev/null || true
    wait "$api_pid" ${web_pid:+"$web_pid"} 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 143' TERM
trap 'exit 130' INT

for (( attempt = 0; attempt < 40; attempt++ )); do
    if curl --fail --silent --max-time 1 "$BACKEND_URL/readyz" >/dev/null; then
        env -u DATABASE_URL node node_modules/next/dist/bin/next start --hostname 0.0.0.0 &
        web_pid=$!
        wait -n "$api_pid" "$web_pid"
        exit $?
    fi
    kill -0 "$api_pid" 2>/dev/null || break
    sleep 1
done
echo 'Nexa API did not become ready' >&2
exit 1
