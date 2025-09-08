# Battery Logger TUI

The Battery Logger TUI provides real-time visualization of your battery data with intelligent discharge prediction.

## Usage

```bash
./battery-logger tui [options]
```

## Options

- `-window duration`: Time window to display and analyze (default: 2h)
  - Examples: `10m`, `30m`, `1h`, `2h`, `4h`
- `-alpha float`: Exponential decay factor for weighted regression (default: 0.05)
  - Higher values give more weight to recent data points
  - Lower values consider historical data more equally
- `-refresh duration`: UI refresh interval (default: 5s)
  - Examples: `1s`, `2s`, `5s`, `10s`

## Features

### 📊 Real-time Visualization
- **Line graph** showing battery percentage over time
- **Color-coded data points**:
  - 🟢 **Green**: When AC is plugged in
  - 🔴 **Red**: When running on battery
- **Dynamic scaling** based on your data range

### 🧮 Smart Predictions
- **Discharge rate calculation** using weighted linear regression
- **Time-to-empty estimation** based on current discharge patterns
- **Rolling window analysis** focusing on recent unplugged sessions
- **Confidence indicators** showing sample size used for predictions

### 📈 Data Insights
- **Sample counts** for AC and battery modes
- **Time span** of displayed data
- **Real-time status** showing current AC/battery state
- **Data file location** for verification

### ⌨️ Controls
- **q** or **Ctrl+C**: Quit the application
- **r**: Force refresh display
- **Window resize**: Automatically adjusts layout

## Examples

### Basic usage with default settings
```bash
./battery-logger tui
```

### Focus on recent data with faster updates
```bash
./battery-logger tui -window 30m -refresh 2s
```

### Long-term analysis with more historical weight
```bash
./battery-logger tui -window 4h -alpha 0.02
```

### High-frequency monitoring
```bash
./battery-logger tui -window 1h -refresh 1s -alpha 0.1
```

## Understanding the Predictions

The TUI uses **weighted linear regression** to predict battery discharge:

1. **Only unplugged sessions** are used for discharge prediction
2. **Recent data points** have higher weight in the calculation
3. **Alpha parameter** controls how quickly weights decay over time
4. **Time-to-empty** is calculated by extrapolating the current discharge rate

### Prediction Accuracy
- **More reliable** with longer unplugged sessions
- **Less reliable** with frequent AC connections
- **Best accuracy** when battery behavior is consistent
- **Sample size indicators** help assess confidence

## Troubleshooting

### "No recent data in window"
- Increase the `-window` parameter
- Check if battery-logger is actively collecting data
- Verify data file exists: `~/.local/state/battery-logger/battery.csv`

### "Need ≥2 unplugged samples"
- Wait for more battery-only data points
- Increase the `-window` to include more historical data
- Check if you've had recent unplugged sessions

### Poor predictions
- Adjust `-alpha` parameter (try 0.02-0.1 range)
- Ensure consistent battery usage patterns
- Wait for more unplugged data points
