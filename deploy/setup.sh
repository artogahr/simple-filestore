#!/usr/bin/env bash
# Run once on the server to install simple-filestore and set it up as a systemd service.
# Usage: bash setup.sh

set -euo pipefail

REPO="artogahr/simple-filestore"
BINARY="/usr/local/bin/simple-filestore"
SERVICE="/etc/systemd/system/simple-filestore.service"
RAW="https://raw.githubusercontent.com/${REPO}/main"
RELEASE_URL="https://github.com/${REPO}/releases/download/latest/simple-filestore"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

log "=== simple-filestore setup ==="

# Download binary
log "Downloading binary..."
curl -fL --progress-bar "$RELEASE_URL" -o "$BINARY"
chmod +x "$BINARY"
log "Installed: $("$BINARY" --version)"

# Download and enable systemd service
log "Installing systemd service..."
curl -fsSL "${RAW}/deploy/simple-filestore.service" -o "$SERVICE"
systemctl daemon-reload
systemctl enable --now simple-filestore
log "Service enabled and started."

# Install 'update' alias
log "Installing 'update' alias..."
echo "alias update='curl -fsSL https://raw.githubusercontent.com/${REPO}/main/deploy/update.sh | bash'" >> /root/.bashrc
log "Added 'update' alias to /root/.bashrc — run 'source /root/.bashrc' or open a new shell."

log ""
systemctl status simple-filestore --no-pager -l | sed 's/^/    /'
log ""
log "=== Setup complete. Visit http://$(hostname -I | awk '{print $1}'):8080 ==="
