#!/bin/bash
#
# Example systemd deployment script for sing-box-agent.
#
# Prerequisites:
#   - Linux host with systemd
#   - sing-box already installed and its systemd unit present (`sing-box.service`)
#   - A prebuilt `sing-box-agent` binary in the current directory
#     (or set BINARY_SRC to point to it)
#
# Usage:  sudo ./deploy.sh
#
set -euo pipefail

BINARY_SRC="${BINARY_SRC:-./sing-box-agent}"
AGENT_CONFIG_SRC="${AGENT_CONFIG_SRC:-./agent-config.yaml}"
SINGBOX_CONFIG_SRC="${SINGBOX_CONFIG_SRC:-./sing-box-config.json}"

AGENT_CONFIG_DEST="/etc/sing-box-agent/config.yaml"
SINGBOX_CONFIG_DEST="/etc/sing-box/config.json"
BINARY_DEST="/usr/local/bin/sing-box-agent"
SERVICE_FILE="/etc/systemd/system/sing-box-agent.service"

if [[ $EUID -ne 0 ]]; then
  echo "This script must be run as root" >&2
  exit 1
fi

for f in "$BINARY_SRC" "$AGENT_CONFIG_SRC" "$SINGBOX_CONFIG_SRC"; do
  if [[ ! -f "$f" ]]; then
    echo "Missing required file: $f" >&2
    exit 1
  fi
done

# 1. Back up any existing sing-box config.
if [[ -f "$SINGBOX_CONFIG_DEST" ]]; then
  backup="${SINGBOX_CONFIG_DEST}.bak.$(date +%Y%m%d%H%M%S)"
  cp "$SINGBOX_CONFIG_DEST" "$backup"
  echo "Backed up existing sing-box config to $backup"
fi

# 2. Install configs.
install -D -m 0644 "$SINGBOX_CONFIG_SRC" "$SINGBOX_CONFIG_DEST"
install -D -m 0600 "$AGENT_CONFIG_SRC" "$AGENT_CONFIG_DEST"

# 3. Install binary.
install -D -m 0755 "$BINARY_SRC" "$BINARY_DEST"

# 4. Create systemd unit.
cat > "$SERVICE_FILE" <<'EOF'
[Unit]
Description=sing-box management agent
After=network-online.target sing-box.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sing-box-agent -config /etc/sing-box-agent/config.yaml
Restart=on-failure
RestartSec=5
User=root
# Sandboxing (tighten to your environment)
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/etc/sing-box /etc/sing-box-agent /var/lib/sing-box-agent
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

# 5. Reload systemd and (re)start services.
systemctl daemon-reload
systemctl enable --now sing-box sing-box-agent

# 6. Verify.
sleep 1
systemctl --no-pager status sing-box-agent
echo
echo "Health check:"
curl -sS --fail http://127.0.0.1:8080/healthz && echo
echo "Deployment complete."
