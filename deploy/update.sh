#!/usr/bin/env bash
# Run on the server to pull the latest build and restart the service.
# Usage: bash update.sh

set -e

REPO="artogahr/simple-filestore"
BINARY="/usr/local/bin/simple-filestore"
SERVICE="/etc/systemd/system/simple-filestore.service"
RAW="https://raw.githubusercontent.com/${REPO}/main"

echo "Downloading latest binary..."
curl -sL "https://github.com/${REPO}/releases/download/latest/simple-filestore" \
  -o /tmp/simple-filestore
chmod +x /tmp/simple-filestore
mv /tmp/simple-filestore "$BINARY"

echo "Updating service file..."
curl -sL "${RAW}/deploy/simple-filestore.service" -o "$SERVICE"
systemctl daemon-reload

echo "Restarting service..."
systemctl enable --now simple-filestore
systemctl restart simple-filestore
echo "Done."
