#!/bin/bash
# Battery Zen Uninstallation Script

set -e

echo "  Uninstalling Battery Zen..."

# Uninstall everything
make uninstall

echo "  Battery Zen uninstalled!"
echo ""
echo "Note: Log files in ~/.local/state/battery-zen/ are preserved"
echo "Remove them manually if desired: rm -rf ~/.local/state/battery-zen/"
