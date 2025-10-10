#!/bin/bash
# Battery Zen Installation Script

set -e

echo "  Installing Battery Zen..."

# Build, install, setup service, desktop icon, and copy config
make setup

echo "  Battery Zen installed and started!"
echo ""
echo "  Binary: ~/.local/bin/battery-zen"
echo "  Application: ~/.local/share/applications/battery-zen.desktop"
echo "  Config file: ~/.config/battery-zen/config.toml"



# Check for missing config fields
config_file="$HOME/.config/battery-zen/config.toml"
default_config="internal/config/config.toml"
required_fields=($(grep -v '^#' "$default_config" | grep " = " | awk -F' = ' '{print $1}' | tr -d ' '))
missing_fields=()
for field in "${required_fields[@]}"; do
    if ! grep -q "^$field = " "$config_file"; then
        missing_fields+=("$field")
    fi
done
if [ ${#missing_fields[@]} -gt 0 ]; then
    echo -e "\e[31m  Warning: The following fields are missing from your config.toml: ${missing_fields[*]}\e[0m"
    echo -e "\e[31mPlease add them and read the README for more info on the missing fields.\e[0m"
else
    echo "  Config file is up to date."
fi
echo "You can edit the above config file to change settings."
