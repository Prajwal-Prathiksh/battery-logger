
<div align="center">
-lo	<img src="assets/battery-zen.png" alt="Battery Zen Logo" width="120" />
</div>

# Battery Zen

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/github/license/Prajwal-Prathiksh/battery-zen?style=flat)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Prajwal-Prathiksh/battery-zen?style=flat)](https://github.com/Prajwal-Prathiksh/battery-zen/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/Prajwal-Prathiksh/battery-zen)](https://goreportcard.com/report/github.com/Prajwal-Prathiksh/battery-zen)
[![Platform](https://img.shields.io/badge/platform-Linux-blue?style=flat&logo=linux)](https://kernel.org/)
[![Systemd](https://img.shields.io/badge/systemd-supported-green?style=flat)](https://systemd.io/)

*A zen-like Go toolkit for mindful battery monitoring, logging, and real-time visualization on Linux.*

<div align="center">
	<img src="assets/battery-zen-tui-v7-screenshot.png" alt="Battery Zen TUI Screenshot" width="480" />
</div>


## Features

- Battery and power monitoring
- Configurable logging intervals
- Automatic log rotation
- Systemd integration
- **Interactive TUI**: real-time charts, predictions, zoom/pan, cycle count
- **Screen-On Time (SOT) tracking** - estimates daily usage patterns
- **Suspend/shutdown detection** - identifies system sleep periods and battery drain
- **Weekly SOT visualization** - bar charts showing daily usage trends


## Installation

```bash
./install.sh
```


## Usage

```bash
battery-zen run      # Start daemon
battery-zen tui      # Launch TUI
battery-zen status   # Show status
```

See [docs/TUI.md](docs/TUI.md) for advanced TUI features and controls.


## Service Management

```bash
make status     # Service status
make logs       # View logs
make stop       # Stop service
make start      # Start service
make uninstall  # Remove everything
```


## Configuration

Config files (TOML):
- [`internal/config/config.toml`](internal/config/config.toml) (local)
- `~/.config/battery-zen/config.toml` (user)
- `/etc/battery-zen/config.toml` (system)

Key settings:
- `interval_secs`: Data logging frequency (default: 60s)
- `suspend_gap_minutes`: Threshold for detecting suspend events (default: 5 min)
- `max_window_zoom`: Chart zoom limit in days (default: 10)
- Chart colors, log rotation, and timezone settings

See config file for all available options.



## Output

CSV log: `~/.local/state/battery-zen/logs.csv`


## Analytics & Predictions

The TUI provides comprehensive battery analytics:

- **Charge/Discharge Rates**: Calculated using exponential weighted regression (recent data weighted higher)
- **Time Estimates**: Predicts time to full charge or empty based on current usage patterns
- **SOT Calculation**: Estimates screen-on time by analyzing gaps in data logging (≥5 minutes = suspend/shutdown, configurable)
- **Current Session**: Active time since last wake/boot
- **Daily Trends**: Bar chart showing SOT for the past 7 days
- **Suspend Detection**: Tracks sleep periods and battery drain during suspend

> **Note**: SOT is calculated as a proxy based on continuous data logging. If the system is left idle with the screen off but Battery Zen still running, it will count toward SOT. The calculation assumes logging gaps ≥5 minutes indicate system suspend/shutdown.


## Manual Service Installation

See [`systemd/`](systemd/) for service files. Use `make` or copy manually for custom setups.


## Uninstallation

```bash
./uninstall.sh
```


## Development

```bash
go build ./cmd/battery-zen
make clean
```


## Configuration Reference

All configuration options available in config files:

### Core Settings
- `interval_secs = 60` - Data logging frequency in seconds
- `interval_secs_on_ac = 60` - Logging frequency when AC connected
- `timezone = "Local"` - Timezone for timestamps
- `log_dir = "~/.local/state/battery-zen"` - Directory for log files
- `log_file = "logs.csv"` - Name of the CSV log file
- `max_lines = 4000` - Maximum lines in log before rotation
- `trim_buffer = 100` - Lines to keep when trimming log
- `max_charge_percent = 100` - Maximum charge threshold for predictions
- `suspend_gap_minutes = 5` - Gap threshold for detecting suspend/shutdown events

### TUI Settings
- `day_color_number = -1` - Terminal color for day data points (default foreground)
- `night_color_number = 234` - Terminal color for night data points (dark gray)
- `day_start_hour = 7` - Hour when day visualization starts (7 AM)
- `day_end_hour = 19` - Hour when night visualization starts (7 PM)
- `max_window_zoom = 10` - Maximum zoom window in days for charts
