#!/usr/bin/env bash
# Run once on the LXC after first boot to set up the service and GitHub Actions runner.
# Usage: bash setup.sh <github-repo-url> <runner-token>
# Example: bash setup.sh https://github.com/youruser/simple-filestore AABBCC...

set -e

REPO_URL="$1"
RUNNER_TOKEN="$2"

if [[ -z "$REPO_URL" || -z "$RUNNER_TOKEN" ]]; then
  echo "Usage: $0 <github-repo-url> <runner-token>"
  exit 1
fi

# Create service user and workspace
useradd -r -s /sbin/nologin -d /var/lib/simple-filestore simple-filestore
mkdir -p /var/lib/simple-filestore
chown simple-filestore:simple-filestore /var/lib/simple-filestore

# Install systemd service (assumes binary already placed at /usr/local/bin/simple-filestore)
cp simple-filestore.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable simple-filestore

# Allow the runner to move the binary and restart the service
cat > /etc/sudoers.d/github-runner <<'EOF'
github-runner ALL=(ALL) NOPASSWD: /usr/bin/mv /home/github-runner/actions-runner/_work/*/*/simple-filestore /usr/local/bin/simple-filestore
github-runner ALL=(ALL) NOPASSWD: /bin/systemctl restart simple-filestore
EOF

# Install GitHub Actions runner
useradd -m -s /bin/bash github-runner
cd /home/github-runner
mkdir actions-runner && cd actions-runner

RUNNER_VERSION=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep '"tag_name"' | sed 's/.*"v\([^"]*\)".*/\1/')
curl -sL "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" | tar xz

chown -R github-runner:github-runner /home/github-runner/actions-runner

# Register the runner
sudo -u github-runner ./config.sh \
  --url "$REPO_URL" \
  --token "$RUNNER_TOKEN" \
  --name "$(hostname)" \
  --labels "deploy" \
  --unattended \
  --replace

# Install as systemd service
./svc.sh install github-runner
./svc.sh start

echo "Done. Runner is registered and running."
echo "Start the app: systemctl start simple-filestore"
