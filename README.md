# Battery Logger

![Battery Logger TUI Screenshot](assets/battery-logger-tui-v4-screenshot.png)

A lightweight Go daemon that logs battery status and power information to CSV files on Linux systems, and provides an interactive TUI for real-time data visualization, discharge prediction, and advanced day/night charting!


## Features

- Continuous battery monitoring with configurable intervals
- Different logging intervals for AC vs battery power
- Automatic log rotation when files grow too large
- Single-instance protection with PID files
- Systemd service integration
- XDG Base Directory compliant
- **Interactive TUI** for real-time data visualization, discharge prediction, and advanced charting
	- Day/night background visualization with configurable colors and hours
		- Interactive zoom and pan controls (mouse wheel or i/o keys to zoom, arrow keys to pan)
	- Battery cycle count display (if available)
	- Real-time status and prediction panel

## Installation
```bash
# Using the install script
./install.sh
```

## Usage

### Commands
```bash
battery-logger sample   # Take one sample
battery-logger run      # Run as daemon
battery-logger status   # Show current status
battery-logger trim     # Trim log to max lines
battery-logger tui      # Launch interactive TUI for data visualization
```


### TUI (Terminal User Interface)
The TUI provides real-time visualization of battery data with intelligent discharge prediction and advanced charting:

```bash
# Basic usage
battery-logger tui

# Focus on recent data with custom settings
battery-logger tui -alpha 0.1
```

**Features:**
- 📊 Interactive time-based chart with day/night backgrounds and zoom/pan support (use mouse wheel or i/o keys to zoom, arrow keys to pan)
- 🧮 Smart discharge/charge rate calculation using weighted regression
- ⏱️ Time-to-empty/full predictions based on recent unplugged/charging sessions
- 📈 Data insights, sample statistics, and battery cycle count (if available)
- ⌨️ Real-time controls: Tab/Shift+Tab to focus, q to quit, r to refresh, mouse wheel or i/o to zoom, arrow keys to pan/scroll

See [docs/TUI.md](docs/TUI.md) for detailed TUI documentation and updated controls/features.

### Service Management
```bash
make status     # Check service status
make logs       # View logs
make stop       # Stop service
make start      # Start service
make uninstall  # Remove everything
```

## Configuration

Configuration files are loaded in priority order:
1. `internal/config/config.toml` (project local)
2. `~/.config/battery-logger/config.toml` (user)
3. `/etc/battery-logger/config.toml` (system)


### Config Options
```toml
interval_secs = 60           # Sample interval on battery
interval_secs_on_ac = 300    # Sample interval on AC power
timezone = "local"           # "local" or "UTC"
log_dir = "~/.local/state/battery-logger"
log_file = "battery.csv"
max_lines = 1000            # Max lines before rotation
trim_buffer = 100           # Buffer for trimming
max_charge_percent = 100     # Maximum charge target (for prediction only)

# Day/Night visualization settings (for TUI chart)
day_color_number = 237       # Terminal color number for day background (0-255)
night_color_number = 0       # Terminal color number for night background (0-255)
day_start_hour = 7           # Hour when day starts (0-23)
day_end_hour = 19            # Hour when night starts (0-23)
```


> [!IMPORTANT]
> The `max_charge_percent` setting is for prediction purposes only. It does not control your battery's actual charging threshold - configure that separately via hardware/system tools. The TUI uses this value for accurate time-to-full predictions.


## Output

Battery data is logged to `~/.local/state/battery-logger/battery.csv`:
```csv
timestamp,ac_online,battery_percent
2025-09-08T17:30:00-05:00,true,85
2025-09-08T17:35:00-05:00,true,84
```

## Manual Service Installation

If you prefer manual systemd setup instead of using `make`:

**User service (recommended):**
```bash
cp systemd/battery-logger@.service ~/.config/systemd/user/battery-logger.service
# Edit ExecStart path if needed
systemctl --user daemon-reload
systemctl --user enable battery-logger.service
systemctl --user start battery-logger.service
```

**System service (requires root):**
```bash
sudo cp systemd/battery-logger.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable battery-logger@$USER.service
sudo systemctl start battery-logger@$USER.service
```

## Uninstallation
```bash
# Stop service and remove everything
./uninstall.sh
```

> [!Note]
> Log files in `~/.local/state/battery-logger/` are preserved by default. Remove them manually if you want to delete your battery usage history.

## Development

```bash
# Build
go build ./cmd/battery-logger

# Test
battery-logger status

# Clean
make clean
```