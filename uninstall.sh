#!/bin/bash
# Battery Logger Uninstallation Script

set -e

echo "🔋 Uninstalling Battery Logger..."

# Uninstall everything
make uninstall

echo "✅ Battery Logger uninstalled!"
echo ""
echo "Note: Log files in ~/.local/state/battery-logger/ are preserved"
echo "Remove them manually if desired: rm -rf ~/.local/state/battery-logger/"
