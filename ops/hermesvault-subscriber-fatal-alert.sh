#!/bin/bash
set -euo pipefail

FATAL_FILE="${HERMESVAULT_SUBSCRIBER_FATAL_FILE:-/home/gws/dev/algorand/hermes/HermesVault-frontend/data/txns/subscriber.fatal}"
HOSTNAME="$(hostname -f 2>/dev/null || hostname)"

if [[ ! -f "$FATAL_FILE" ]]; then
    logger -t hermesvault-alert "Subscriber fatal marker is not present: $FATAL_FILE"
    exit 0
fi

MESSAGE="$(cat "$FATAL_FILE" 2>/dev/null || echo "subscriber.fatal exists")"

curl -fsS \
    --retry 3 \
    --retry-delay 2 \
    --connect-timeout 10 \
    --data-urlencode "chat_id=${TELEGRAM_CHAT_ID:?}" \
    --data-urlencode "text=HermesVault subscriber fatal marker activated on ${HOSTNAME}

${MESSAGE}" \
    "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN:?}/sendMessage" >/dev/null

logger -t hermesvault-alert "Sent Telegram alert for subscriber fatal marker"
