#!/bin/sh
#
# Entrypoint for the sing-box-agent container.
#
# Starts sing-box in the background (writing its PID to a file so the agent
# can reload it with SIGHUP) and then launches the agent in the foreground.
# tini (PID 1) forwards signals; this script propagates them to both
# processes so `docker stop` shuts the container down cleanly.
#
set -eu

SINGBOX_BIN="${SINGBOX_BIN:-/usr/local/bin/sing-box}"
SINGBOX_CONFIG="${SINGBOX_AGENT_SINGBOX_CONFIG_PATH:-/etc/sing-box/config.json}"
SINGBOX_PID_FILE="${SINGBOX_AGENT_RELOAD_TARGET:-/run/sing-box.pid}"
AGENT_CONFIG="${AGENT_CONFIG:-/etc/sing-box-agent/config.yaml}"

if [ ! -f "$SINGBOX_CONFIG" ]; then
    echo "sing-box config not found at $SINGBOX_CONFIG" >&2
    exit 1
fi

# Start sing-box in the background.
"$SINGBOX_BIN" run -c "$SINGBOX_CONFIG" &
SINGBOX_PID=$!
echo "$SINGBOX_PID" > "$SINGBOX_PID_FILE"
echo "sing-box started (pid=$SINGBOX_PID, config=$SINGBOX_CONFIG)"

# Forward termination signals to both children.
term() {
    echo "entrypoint: forwarding TERM"
    kill -TERM "$SINGBOX_PID" 2>/dev/null || true
    if [ -n "${AGENT_PID:-}" ]; then
        kill -TERM "$AGENT_PID" 2>/dev/null || true
    fi
}
trap term TERM INT

# Start the agent in the foreground, falling back to env-only config when
# no file is mounted.
if [ -f "$AGENT_CONFIG" ]; then
    /usr/local/bin/sing-box-agent -config "$AGENT_CONFIG" &
else
    echo "no agent config at $AGENT_CONFIG; starting from environment only"
    /usr/local/bin/sing-box-agent &
fi
AGENT_PID=$!
echo "sing-box-agent started (pid=$AGENT_PID)"

# Wait for either process to exit and propagate the status.
wait -n "$SINGBOX_PID" "$AGENT_PID"
EXIT_CODE=$?

# One has exited; bring the other down too.
kill -TERM "$SINGBOX_PID" 2>/dev/null || true
kill -TERM "$AGENT_PID" 2>/dev/null || true
wait 2>/dev/null || true

rm -f "$SINGBOX_PID_FILE"
exit "$EXIT_CODE"
