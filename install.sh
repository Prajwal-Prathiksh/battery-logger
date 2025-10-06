#!/bin/bash
# Battery Logger Installation Script

set -e

echo "  Installing Battery Logger..."

# Build, install, setup service, desktop icon, and copy config
make setup

echo "  Battery Logger installed and started!"
echo ""
echo "  Data file: ~/.local/state/battery-logger/battery.csv"
echo "  Binary: ~/.local/bin/battery-logger"
echo "  Application: ~/.local/share/applications/battery-logger.desktop"
echo "  Config file: ~/.config/battery-logger/config.toml"
echo ""
echo "You can edit the above config file to change settings."
