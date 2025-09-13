# Battery Logger TUI

The Battery Logger TUI provides real-time visualization of your battery data with intelligent discharge prediction, advanced charting, and interactive controls.


## Usage

```bash
./battery-logger tui [options]
```

## Options


- `-alpha float`: Exponential decay factor for weighted regression (default: 0.05)
  - Higher values give more weight to recent data points
  - Lower values consider historical data more equally
- **Refresh rate**: Fixed at 10 seconds.

## Features


### 📊 Real-time Visualization
- **Two-pane layout**: interactive chart (70%) and status panel (30%)
- **Day/night background visualization**: configurable colors and hours
- **Interactive time-based chart** with zoom (mouse wheel or i/o keys), pan (←→), and reset (Esc)
- **Color-coded data series**:
  - 🟢 **Green line**: When AC is plugged in
  - 🔴 **Red line**: When running on battery
- **Time-based X-axis** with intelligent labeling and date annotations
- **Real-time status panel** with battery cycle count (if available)

### 🧮 Smart Predictions
- **Discharge rate calculation** using weighted linear regression
- **Time-to-empty estimation** based on current discharge patterns
- **Time-to-full estimation** with configurable charge targets (respects `max_charge_percent` setting)
- **Contiguous session analysis** focusing on most recent unplugged/charging period
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
- **Tab**: Focus next widget
- **Shift+Tab**: Focus previous widget
- **↑/↓**: Scroll info panel up/down
- **Mouse wheel or i/o keys**: Zoom in/out on chart (up/i to zoom in, down/o to zoom out)
- **←/→**: Pan chart left/right
- **Esc**: Reset chart zoom/pan to full data range
- **Window resize**: Automatically adjusts layout

## Examples


### Basic usage with default settings
```bash
./battery-logger tui
```

### Focus on recent data with custom settings
```bash
./battery-logger tui -alpha 0.1
```


## Understanding the Predictions

The TUI uses **weighted linear regression** to predict battery discharge and charging:

1. **Only the most recent contiguous session** is used for prediction (unplugged for discharge, plugged for charging)
2. **Recent data points** have higher weight in the calculation (exponential decay)
3. **Alpha parameter** controls how quickly weights decay over time
4. **Time-to-empty** is calculated using current battery level and discharge rate
5. **Time-to-full** uses the configured maximum charge target (`max_charge_percent` from config)
6. **Transition tracking** identifies when current AC status started
7. **Battery cycle count** is displayed if available from your system


### Algorithm Details
- **Contiguous session analysis**: Walks backward from latest data to find the most recent uninterrupted session (unplugged or plugged)
- **Weighted regression**: Recent samples get exponentially higher weights based on alpha parameter
- **Real-time prediction**: Uses actual current battery level, not regression intercept
- **Configurable charge target**: Time-to-full predictions respect the `max_charge_percent` configuration (e.g., 80% instead of 100%)
- **Status transitions**: Tracks AC plug/unplug events with timestamps and battery levels
- **Day/night chart backgrounds**: Visualize battery usage patterns across day and night


### Prediction Accuracy
- **More reliable** with longer contiguous sessions (≥2 samples required for either charging or discharging)
- **Most accurate** during consistent battery usage or charging patterns
- **Less reliable** immediately after AC transitions
- **Best results** with steady discharge/charge rates and sufficient session data
- **Charge predictions** depend on configured maximum charge target and charging behavior


## Troubleshooting

### "No recent data in window"
- Check if battery-logger service is running: `systemctl --user status battery-logger`
- Verify data file exists: `~/.local/state/battery-logger/battery.csv`
- Ensure battery-logger has been collecting data for some time

### "Need ≥2 charging/discharging samples"
- Wait for more data points to be collected in the current session (charging or discharging)
- Check if you've had recent sessions longer than the sampling interval
- Verify the service is actively logging during both plugged and unplugged periods
- For charging predictions: ensure you have sufficient charging session data
- For discharge predictions: ensure you have sufficient unplugged session data

### "∞ (not discharging or charging)" or poor predictions
- This appears when discharge rate is near zero or positive
- Ensure you're actually using the battery (not idle/suspended)
- Try adjusting `-alpha` parameter (range: 0.02-0.1)
- Wait for more consistent battery usage patterns
- Check if power management is affecting discharge rates