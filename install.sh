#!/bin/bash
# Battery Logger Installation Script

set -e

echo "  Installing Battery Logger..."

# Build and install
make setup
make desktop-icon

echo "  Battery Logger installed and started!"
echo ""
echo "Useful commands:"
echo "  make status    - Check service status"
echo "  make logs      - View logs"
echo "  make stop      - Stop service"
echo "  make start     - Start service"
echo "  make uninstall - Remove everything"
echo ""
echo "Log file: ~/.local/state/battery-logger/battery.csv"
echo "Binary: ~/.local/bin/battery-logger"
echo "Desktop icon: ~/.local/share/applications/battery-logger.desktop"
