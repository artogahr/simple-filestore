#!/usr/bin/env bash
# Run on the server to pull the latest build and restart the service.
# Usage: bash update.sh

set -e

REPO="artogahr/simple-filestore"
BINARY="/usr/local/bin/simple-filestore"

echo "Downloading latest build..."
curl -sL "https://github.com/${REPO}/releases/download/latest/simple-filestore" \
  -o /tmp/simple-filestore

chmod +x /tmp/simple-filestore
mv /tmp/simple-filestore "$BINARY"

echo "Restarting service..."
systemctl restart simple-filestore
echo "Done."
