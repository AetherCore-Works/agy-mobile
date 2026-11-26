#!/bin/sh
set -eu

TTYD_PORT="${TTYD_PORT:-7681}"
TTYD_BASE_PATH="${TTYD_BASE_PATH:-/terminal}"
TTYD_SSH_TARGET="${TTYD_SSH_TARGET:-user@192.0.2.10}"
TTYD_SSH_PORT="${TTYD_SSH_PORT:-22}"
TTYD_REMOTE_SHELL="${TTYD_REMOTE_SHELL:-bash}"
TTYD_AUTO_AUTH="${TTYD_AUTO_AUTH:-true}"
TTYD_SSH_KEY="${TTYD_SSH_KEY:-/root/.ssh/id_ed25519}"

set -- ssh -t \
  -p "${TTYD_SSH_PORT}" \
  -o StrictHostKeyChecking=accept-new \
  -o UserKnownHostsFile=/tmp/ttyd_known_hosts \
  -o LogLevel=ERROR

if [ "${TTYD_AUTO_AUTH}" = "true" ] && [ -r "${TTYD_SSH_KEY}" ]; then
  cp "${TTYD_SSH_KEY}" /tmp/.ttyd_key
  chmod 600 /tmp/.ttyd_key
  set -- "$@" -i /tmp/.ttyd_key
fi

exec ttyd -p "${TTYD_PORT}" -b "${TTYD_BASE_PATH}" -W -t fontSize=14 "$@" "${TTYD_SSH_TARGET}" "exec ${TTYD_REMOTE_SHELL} -l"
