#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "building jankgps..."
cd "$SCRIPT_DIR"
make build

echo "installing binary to /usr/local/bin/"
install -m 0755 bin/jankgps /usr/local/bin/jankgps

echo "installing systemd service..."
install -m 0644 jankgps.service /etc/systemd/system/jankgps.service
systemctl daemon-reload
systemctl enable jankgps

echo "installing ts2phc config..."
install -m 0644 ts2phc.conf /etc/ts2phc.conf

echo "done. start with: systemctl start jankgps"
echo "logs: journalctl -u jankgps -f"
echo "ts2phc pty will be at /run/jankgps/ts2phc"
