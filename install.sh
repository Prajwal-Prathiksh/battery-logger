#!/bin/bash
# Battery Logger Installation Script

set -e

echo "  Installing Battery Logger..."

# Build and install
make setup
make desktop-icon
make copy-config

echo "  Battery Logger installed and started!"
echo ""
echo "  Data file: ~/.local/state/battery-logger/battery.csv"
echo "  Binary: ~/.local/bin/battery-logger"
echo "  Application: ~/.local/share/applications/battery-logger.desktop"
echo "  Config file: ~/.config/battery-logger/config.toml"
echo ""
echo "You can edit the config file to change settings."
