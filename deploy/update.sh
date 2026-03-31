#!/usr/bin/env bash
# Run on the server to pull the latest build and restart the service.
# Usage: bash update.sh

set -euo pipefail

REPO="artogahr/simple-filestore"
BINARY="/usr/local/bin/simple-filestore"
SERVICE="/etc/systemd/system/simple-filestore.service"
RAW="https://raw.githubusercontent.com/${REPO}/main"
RELEASE_URL="https://github.com/${REPO}/releases/download/latest/simple-filestore"

log() { echo "[$(date '+%H:%M:%S')] $*"; }
die() { echo "[$(date '+%H:%M:%S')] ERROR: $*" >&2; exit 1; }

log "=== simple-filestore update ==="

# Show currently running version
if [[ -x "$BINARY" ]]; then
    CURRENT_VERSION=$("$BINARY" --version 2>/dev/null || echo "unknown")
    log "Current version : $CURRENT_VERSION"
else
    log "No existing binary at $BINARY"
    CURRENT_VERSION=""
fi

# Download new binary to a temp file
TMP=$(mktemp /tmp/simple-filestore.XXXXXX)
trap "rm -f '$TMP'" EXIT

log "Downloading from GitHub Releases..."
HTTP_CODE=$(curl -fL --progress-bar -w "%{http_code}" "$RELEASE_URL" -o "$TMP")
[[ "$HTTP_CODE" == "200" ]] || die "Download failed (HTTP $HTTP_CODE)"
chmod +x "$TMP"

# Verify the new binary runs and show its version
NEW_VERSION=$("$TMP" --version 2>/dev/null) || die "Downloaded binary failed to run --version"
log "New version     : $NEW_VERSION"

# Skip if unchanged
if [[ "$NEW_VERSION" == "$CURRENT_VERSION" ]] && cmp -s "$BINARY" "$TMP" 2>/dev/null; then
    log "Already up to date — nothing to do."
    exit 0
fi

# Install
log "Installing new binary..."
mv "$TMP" "$BINARY"
trap - EXIT  # clear temp-file cleanup since we moved it
log "Binary installed at $BINARY"

# Update service file
log "Updating systemd service file..."
curl -fsSL "${RAW}/deploy/simple-filestore.service" -o "$SERVICE" \
    || die "Failed to download service file"
systemctl daemon-reload
log "Service file updated."

# Restart
log "Restarting service..."
systemctl enable --now simple-filestore
systemctl restart simple-filestore

# Wait briefly then show status
sleep 1
if systemctl is-active --quiet simple-filestore; then
    log "Service is running."
else
    log "WARNING: service does not appear to be running!"
fi

log ""
systemctl status simple-filestore --no-pager -l | sed 's/^/    /'

log ""
log "=== Update complete: $CURRENT_VERSION -> $NEW_VERSION ==="
