#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="/etc/hermesvault-alert.env"
SERVICE_NAME="hermesvault-subscriber-fatal-alert.service"
PATH_NAME="hermesvault-subscriber-fatal-alert.path"

if [[ ! -f "$ENV_FILE" ]]; then
    echo "Missing $ENV_FILE"
    echo "Create it from $SCRIPT_DIR/hermesvault-alert.env.example before installing."
    exit 1
fi

sudo install -m 0644 "$SCRIPT_DIR/$SERVICE_NAME" "/etc/systemd/system/$SERVICE_NAME"
sudo install -m 0644 "$SCRIPT_DIR/$PATH_NAME" "/etc/systemd/system/$PATH_NAME"

sudo systemctl daemon-reload
sudo systemctl enable --now "$PATH_NAME"
sudo systemctl status --no-pager "$PATH_NAME"
