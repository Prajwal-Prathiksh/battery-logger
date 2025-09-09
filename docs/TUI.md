# Battery Logger TUI

The Battery Logger TUI provides real-time visualization of your battery data with intelligent discharge prediction.

## Usage

```bash
./battery-logger tui [options]
```

## Options

- `-window duration`: Time window to display and analyze (default: 10h)
  - Examples: `10m`, `30m`, `1h`, `2h`, `4h`, `6h`, `10h`
  - Can be changed in real-time via input field
- `-alpha float`: Exponential decay factor for weighted regression (default: 0.05)
  - Higher values give more weight to recent data points
  - Lower values consider historical data more equally
  - Can be changed in real-time via input field
- **Refresh rate**: Fixed at 10 seconds.

## Features

### 📊 Real-time Visualization
- **Three-pane layout** with graph (60%), status info, and controls
- **Interactive line chart** with mouse zoom support
- **Color-coded data series**:
  - 🟢 **Green line**: When AC is plugged in
  - 🔴 **Red line**: When running on battery
- **Time-based X-axis** with intelligent labeling
- **Real-time parameter controls** with auto-refresh on changes
- **Smart data handling** with NaN gaps to maintain time positioning

### 🧮 Smart Predictions
- **Discharge rate calculation** using weighted linear regression
- **Time-to-empty estimation** based on current discharge patterns
- **Contiguous session analysis** focusing on most recent unplugged period
- **Confidence indicators** showing sample size used for predictions
- **AC transition tracking** with time and battery level when status changed

### 📈 Data Insights
- **Current status** with AC connection state and battery percentage
- **Transition history** showing when current AC status started
- **Sample counts** for AC and battery modes within the window
- **Time span** of displayed data with start/end times
- **Data file location** and configuration file paths
- **Real-time discharge rate** in %/min

### ⌨️ Controls
- **q** or **Q**: Quit the application
- **r** or **R**: Force refresh display
- **Tab**: Focus next input field
- **Shift+Tab**: Focus previous input field
- **Enter**: Apply changes in input fields (auto-refreshes data)
- **↑/↓**: Scroll info panel up/down
- **Mouse wheel**: Zoom in/out on chart (up to zoom in, down to zoom out)
- **Window resize**: Automatically adjusts layout

## Examples

### Basic usage with default settings
```bash
./battery-logger tui
```

### Focus on recent data with custom settings
```bash
./battery-logger tui -window 30m -alpha 0.1
```

### Long-term analysis with more historical weight
```bash
./battery-logger tui -window 4h -alpha 0.02
```

### High-frequency monitoring
```bash
./battery-logger tui -window 1h -alpha 0.1
```

## Understanding the Predictions

The TUI uses **weighted linear regression** to predict battery discharge:

1. **Only the most recent contiguous unplugged session** is used for discharge prediction
2. **Recent data points** have higher weight in the calculation (exponential decay)
3. **Alpha parameter** controls how quickly weights decay over time
4. **Time-to-empty** is calculated using current battery level and discharge rate
5. **Transition tracking** identifies when current AC status started

### Algorithm Details
- **Contiguous session analysis**: Walks backward from latest data to find the most recent uninterrupted unplugged period
- **Weighted regression**: Recent samples get exponentially higher weights based on alpha parameter
- **Real-time prediction**: Uses actual current battery level, not regression intercept
- **Status transitions**: Tracks AC plug/unplug events with timestamps and battery levels

### Prediction Accuracy
- **More reliable** with longer unplugged sessions (≥2 samples required)
- **Most accurate** during consistent battery usage patterns
- **Less reliable** immediately after AC transitions
- **Best results** with steady discharge rates and sufficient unplugged data

## Troubleshooting

### "No recent data in window"
- Increase the `-window` parameter (try `-window 12h` or `-window 1d`)
- Check if battery-logger service is running: `systemctl --user status battery-logger`
- Verify data file exists: `~/.local/state/battery-logger/battery.csv`
- Ensure battery-logger has been collecting data for some time

### "Need ≥2 unplugged samples"
- Wait for more battery-only data points to be collected
- Increase the `-window` to include more historical data
- Check if you've had recent unplugged sessions longer than the sampling interval
- Verify the service is actively logging during unplugged periods

### "∞ (not discharging or charging)" or poor predictions
- This appears when discharge rate is near zero or positive
- Ensure you're actually using the battery (not idle/suspended)
- Try adjusting `-alpha` parameter (range: 0.02-0.1)
- Wait for more consistent battery usage patterns
- Check if power management is affecting discharge rates