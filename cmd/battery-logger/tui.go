package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/config"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/sysfs"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/mum4k/termdash/widgets/text"
	"github.com/mum4k/termdash/widgets/textinput"
)

// UIParams holds the real-time adjustable parameters
type UIParams struct {
	Window  time.Duration
	Alpha   float64
	Refresh time.Duration
	mu      sync.RWMutex
}

// Get returns thread-safe copies of the parameters
func (p *UIParams) Get() (time.Duration, float64, time.Duration) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Window, p.Alpha, p.Refresh
}

// SetWindow sets the window parameter thread-safely
func (p *UIParams) SetWindow(window time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Window = window
}

// SetAlpha sets the alpha parameter thread-safely
func (p *UIParams) SetAlpha(alpha float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Alpha = alpha
}

// StatusInfo holds information needed for status display
type StatusInfo struct {
	Latest           analytics.Row
	TransitionTime   time.Time
	TransitionBatt   float64
	RateLabel        string
	SlopeStr         string
	Confidence       string
	Estimate         string
	TotalSamples     int
	ACSamples        int
	BattSamples      int
	TimeRange        time.Duration
	StartTime        string
	EndTime          string
	ConfigStr        string
	CurrentWindow    time.Duration
	LogPath          string
	MaxChargePercent int
	CycleCount       int
	HasCycleCount    bool
}

// createChartWidget creates and configures the line chart widget
func createChartWidget() (*linechart.LineChart, error) {
	return linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorWhite)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorCyan)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorCyan)),
		linechart.YAxisCustomScale(0, 100),
		linechart.YAxisFormattedValues(linechart.ValueFormatterRoundWithSuffix("%")),
		linechart.ZoomStepPercent(5),
	)
}

// createTextWidget creates and configures the text display widget
func createTextWidget() (*text.Text, error) {
	return text.New(text.WrapAtWords())
}

// createInputWidgets creates the parameter input widgets with callbacks
func createInputWidgets(windowStr string, alpha float64, uiParams *UIParams, updateData *func() error) (*textinput.TextInput, *textinput.TextInput, error) {
	windowInput, err := textinput.New(
		textinput.Label("Window (time span to show): ", cell.FgColor(cell.ColorCyan)),
		textinput.DefaultText(windowStr),
		textinput.MaxWidthCells(12),
		textinput.PlaceHolder("e.g., 6h, 30m"),
		textinput.OnSubmit(func(text string) error {
			if d, err := time.ParseDuration(text); err == nil {
				uiParams.SetWindow(d)
				// Auto-refresh data with new window setting
				if *updateData != nil {
					if err := (*updateData)(); err != nil {
						log.Printf("Auto-refresh after window change error: %v", err)
					}
				}
			}
			return nil
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating window input: %v", err)
	}

	alphaInput, err := textinput.New(
		textinput.Label("Alpha (decay rate/min): ", cell.FgColor(cell.ColorCyan)),
		textinput.DefaultText(fmt.Sprintf("%.3f", alpha)),
		textinput.MaxWidthCells(8),
		textinput.PlaceHolder("0.001-1.0"),
		textinput.OnSubmit(func(text string) error {
			if a, err := strconv.ParseFloat(text, 64); err == nil && a > 0 && a <= 1 {
				uiParams.SetAlpha(a)
				// Auto-refresh data with new alpha setting
				if *updateData != nil {
					if err := (*updateData)(); err != nil {
						log.Printf("Auto-refresh after alpha change error: %v", err)
					}
				}
			}
			return nil
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating alpha input: %v", err)
	}

	return windowInput, alphaInput, nil
}

// createUILayout creates the TUI container layout with all widgets
func createUILayout(t terminalapi.Terminal, chartWidget *linechart.LineChart, textWidget *text.Text, windowInput, alphaInput *textinput.TextInput) (*container.Container, error) {
	return container.New(
		t,
		container.Border(linestyle.Light),
		container.BorderTitle("Battery Logger TUI - Tab/Shift+Tab: focus, Enter: apply changes, q: quit, r: refresh"),
		container.KeyFocusNext(keyboard.KeyTab),
		container.KeyFocusPrevious(keyboard.KeyBacktab),
		container.SplitHorizontal(
			container.Top(
				container.Border(linestyle.Light),
				container.BorderTitle("Battery % Over Time - Mouse up/down to zoom"),
				container.PlaceWidget(chartWidget),
			),
			container.Bottom(
				container.SplitHorizontal(
					container.Top(
						container.Border(linestyle.Light),
						container.BorderTitle("Battery Status & Prediction - ↑↓ to scroll"),
						container.PlaceWidget(textWidget),
					),
					container.Bottom(
						container.Border(linestyle.Light),
						container.BorderTitle("Settings - Press Enter to apply"),
						container.SplitVertical(
							container.Left(
								container.PlaceWidget(windowInput),
							),
							container.Right(
								container.PlaceWidget(alphaInput),
							),
							container.SplitPercent(50),
						),
					),
					container.SplitFixedFromEnd(4),
				),
			),
			container.SplitPercent(60),
		),
	)
}

// processChartData handles all the data processing for chart display
func processChartData(rows []analytics.Row, window time.Duration) ([]float64, []float64, map[int]string, bool, bool, error) {
	if len(rows) == 0 {
		return nil, nil, nil, false, false, fmt.Errorf("no data available")
	}

	// Create time-based bins (1-minute bins for finest granularity)
	binSize := 1 * time.Minute
	bins := binDataToTimeGrid(rows, binSize, window)
	dataStartTime := rows[0].T
	acSeries, battSeries, labels := createTimeBasedSeries(bins, dataStartTime)

	// Check for data presence and prepare values
	var hasAC, hasBatt bool
	acValues := make([]float64, len(acSeries))
	battValues := make([]float64, len(battSeries))

	for i := range acSeries {
		if !math.IsNaN(acSeries[i]) {
			hasAC = true
		}
		if !math.IsNaN(battSeries[i]) {
			hasBatt = true
		}
		acValues[i] = acSeries[i]
		battValues[i] = battSeries[i]
	}

	return acValues, battValues, labels, hasAC, hasBatt, nil
}

// updateChartWidget updates the chart widget with new data
func updateChartWidget(chartWidget *linechart.LineChart, acValues, battValues []float64, labels map[int]string, hasAC, hasBatt bool) error {
	// Clear previous chart data
	chartWidget.Series("charging", nil)
	chartWidget.Series("discharging", nil)

	// Add series to chart with time-based labels
	if hasAC {
		if err := chartWidget.Series("charging", acValues,
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorGreen), cell.BgColor(cell.ColorDefault)),
			linechart.SeriesXLabels(labels),
		); err != nil {
			return fmt.Errorf("setting AC series: %v", err)
		}
	}

	if hasBatt {
		if err := chartWidget.Series("discharging", battValues,
			linechart.SeriesCellOpts(cell.FgColor(cell.ColorRed), cell.BgColor(cell.ColorDefault)),
			linechart.SeriesXLabels(labels),
		); err != nil {
			return fmt.Errorf("setting battery series: %v", err)
		}
	}

	return nil
}

// generateStatusInfo processes battery data to create status information
func generateStatusInfo(rows []analytics.Row, alpha float64, uiParams *UIParams, logPath string, cfg config.Config) StatusInfo {
	latest := rows[len(rows)-1]

	// Find when the current AC status started
	transitionTime, transitionBatt := findLastACTransition(rows)

	// For regression, consider only the most recent contiguous samples with the same AC state
	currentACState := latest.AC
	contiguousSamples := analytics.FilterContiguousACState(rows, currentACState)

	var est string
	var slopeStr string
	var confidence string
	var rateLabel string

	if len(contiguousSamples) >= 2 {
		rate, estimateMins, conf, ok := analytics.CalculateRateAndEstimate(contiguousSamples, latest.Batt, alpha, cfg.MaxChargePercent)

		if ok {
			if currentACState {
				// Charging mode
				rateLabel = "Charge Rate"
				if rate > 1e-6 {
					est = analytics.FmtDur(estimateMins)
				} else {
					est = "∞ (not charging or already full)"
				}
			} else {
				// Discharging mode
				rateLabel = "Discharge Rate"
				if rate < -1e-6 {
					est = analytics.FmtDur(estimateMins)
				} else {
					est = "∞ (not discharging)"
				}
			}
			slopeStr = fmt.Sprintf("%.3f %%/min", rate)
			confidence = conf
		} else {
			if currentACState {
				rateLabel = "Charge Rate"
			} else {
				rateLabel = "Discharge Rate"
			}
			est = "—"
			slopeStr = "n/a"
			confidence = conf
		}
	} else {
		if currentACState {
			rateLabel = "Charge Rate"
		} else {
			rateLabel = "Discharge Rate"
		}
		est = "—"
		slopeStr = "n/a"
		acStateStr := "charging"
		if !currentACState {
			acStateStr = "discharging"
		}
		confidence = fmt.Sprintf("(need ≥2 %s samples)", acStateStr)
	}

	// Count total samples in window
	totalSamples := len(rows)
	acSamples := 0
	battSamples := 0
	for _, r := range rows {
		if r.AC {
			acSamples++
		} else {
			battSamples++
		}
	}

	// Calculate time range
	timeRange := rows[len(rows)-1].T.Sub(rows[0].T)
	startTime := rows[0].T.Format("15:04")
	endTime := rows[len(rows)-1].T.Format("15:04")

	// Get config file paths
	_, existingConfigPaths := config.GetConfigPaths()
	var configStr string
	if len(existingConfigPaths) == 0 {
		configStr = "📋 Config: Using defaults (no config file found)"
	} else if len(existingConfigPaths) == 1 {
		configStr = fmt.Sprintf("📋 Config file: %s", existingConfigPaths[0])
	} else {
		configStr = fmt.Sprintf("📋 Config files: %s (+ %d more)", existingConfigPaths[len(existingConfigPaths)-1], len(existingConfigPaths)-1)
	}

	// Get current UI parameters for display
	currentWindow, _, _ := uiParams.Get()

	// Get battery cycle count
	cycleCount, hasCycleCount := sysfs.BatteryCycleCount()

	return StatusInfo{
		Latest:           latest,
		TransitionTime:   transitionTime,
		TransitionBatt:   transitionBatt,
		RateLabel:        rateLabel,
		SlopeStr:         slopeStr,
		Confidence:       confidence,
		Estimate:         est,
		TotalSamples:     totalSamples,
		ACSamples:        acSamples,
		BattSamples:      battSamples,
		TimeRange:        timeRange,
		StartTime:        startTime,
		EndTime:          endTime,
		ConfigStr:        configStr,
		CurrentWindow:    currentWindow,
		LogPath:          logPath,
		MaxChargePercent: cfg.MaxChargePercent,
		CycleCount:       cycleCount,
		HasCycleCount:    hasCycleCount,
	}
}

// updateStatusText writes formatted status information to the text widget
func updateStatusText(textWidget *text.Text, info StatusInfo) {
	textWidget.Reset()

	// Latest status
	acStatus := "Unplugged"
	acIcon := "🔋"
	if info.Latest.AC {
		acStatus = "Plugged In"
		acIcon = "🔌"
	}

	var sinceStr string
	if !info.TransitionTime.IsZero() {
		sinceStr = fmt.Sprintf(" (Since: %s, %.1f%%)", info.TransitionTime.Format("15:04"), info.TransitionBatt)
	}

	// Determine time estimation label based on AC state
	var timeDisplayText string
	if info.Latest.AC {
		timeDisplayText = fmt.Sprintf("⏱️  Time to Full (%d%%): %s", info.MaxChargePercent, info.Estimate)
	} else {
		timeDisplayText = fmt.Sprintf("⏱️  Time to Empty (0%%): %s", info.Estimate)
	}

	// Write status information
	statusLines := []string{
		fmt.Sprintf("%s AC Status: %s%s", acIcon, acStatus, sinceStr),
		fmt.Sprintf("🔋 Current Battery: %.1f%%", info.Latest.Batt),
	}

	// Add cycle count if available
	if info.HasCycleCount {
		statusLines = append(statusLines, fmt.Sprintf("🔄 Battery Cycles: %d", info.CycleCount))
	}

	// Continue with rest of the status lines
	remainingLines := []string{
		fmt.Sprintf("📈 %s: %s %s", info.RateLabel, info.SlopeStr, info.Confidence),
		timeDisplayText,
		"",
		fmt.Sprintf("📊 Data Summary (window: %s):", info.CurrentWindow),
		fmt.Sprintf("   Total samples: %d (spanning %s)", info.TotalSamples, info.TimeRange.Round(time.Minute).String()),
		fmt.Sprintf("   AC plugged: %d samples", info.ACSamples),
		fmt.Sprintf("   On battery: %d samples", info.BattSamples),
		fmt.Sprintf("   Time range: %s to %s", info.StartTime, info.EndTime),
		"",
		fmt.Sprintf("📄 Data file: %s", info.LogPath),
		info.ConfigStr,
	}

	statusLines = append(statusLines, remainingLines...)

	for _, line := range statusLines {
		var opts []text.WriteOption
		if strings.Contains(line, "AC plugged") {
			opts = append(opts, text.WriteCellOpts(cell.FgColor(cell.ColorGreen)))
		} else if strings.Contains(line, "On battery") {
			opts = append(opts, text.WriteCellOpts(cell.FgColor(cell.ColorRed)))
		} else if strings.HasPrefix(line, "🔋") || strings.HasPrefix(line, "🔌") {
			opts = append(opts, text.WriteCellOpts(cell.FgColor(cell.ColorYellow)))
		} else if strings.HasPrefix(line, "⚙️") {
			opts = append(opts, text.WriteCellOpts(cell.FgColor(cell.ColorCyan)))
		}

		textWidget.Write(line+"\n", opts...)
	}
}

// setupDataRefresh sets up periodic data refresh and returns the update function
func setupDataRefresh(ctx context.Context, logPath string, uiParams *UIParams, chartWidget *linechart.LineChart, textWidget *text.Text, cfg config.Config) (func() error, error) {
	updateData := func() error {
		window, alpha, _ := uiParams.Get()

		rows, err := readCSV(logPath)
		if err != nil || len(rows) == 0 {
			textWidget.Write(fmt.Sprintf("Could not read data from %s: %v\n", logPath, err), text.WriteCellOpts(cell.FgColor(cell.ColorRed)))
			textWidget.Write("Press q to quit, r to refresh\n")
			return nil
		}

		rows = analytics.FilterWindow(rows, window)
		if len(rows) == 0 {
			textWidget.Write("No recent data in window.\n", text.WriteCellOpts(cell.FgColor(cell.ColorYellow)))
			textWidget.Write("Press q to quit, r to refresh\n")
			return nil
		}

		// Process chart data
		acValues, battValues, labels, hasAC, hasBatt, err := processChartData(rows, window)
		if err != nil {
			return fmt.Errorf("processing chart data: %v", err)
		}

		// Update chart
		if err := updateChartWidget(chartWidget, acValues, battValues, labels, hasAC, hasBatt); err != nil {
			return fmt.Errorf("updating chart: %v", err)
		}

		// Generate and update status text
		statusInfo := generateStatusInfo(rows, alpha, uiParams, logPath, cfg)
		updateStatusText(textWidget, statusInfo)

		return nil
	}

	// Set up periodic refresh
	_, _, currentRefresh := uiParams.Get()
	refreshTicker := time.NewTicker(currentRefresh)

	go func() {
		defer refreshTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-refreshTicker.C:
				if err := updateData(); err != nil {
					log.Printf("Data update error: %v", err)
				}
			}
		}
	}()

	return updateData, nil
}

// createKeyboardHandler creates the keyboard event handler for the TUI
func createKeyboardHandler(cancel context.CancelFunc, updateData func() error) func(*terminalapi.Keyboard) {
	return func(k *terminalapi.Keyboard) {
		if k.Key == 'q' || k.Key == 'Q' {
			cancel()
		}
		if k.Key == 'r' || k.Key == 'R' {
			if err := updateData(); err != nil {
				log.Printf("Manual refresh error: %v", err)
			}
		}
	}
}

// runTUI implements the TUI command using termdash with real-time parameter controls
func runTUI() {
	var windowStr string
	var alpha float64

	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	fs.StringVar(&windowStr, "window", "10h", "rolling window to display & regress (e.g., 10m, 30m, 2h)")
	fs.Float64Var(&alpha, "alpha", 0.05, "exponential decay per minute for weights (e.g., 0.05)")

	if len(os.Args) > 2 {
		fs.Parse(os.Args[2:])
	}

	window, err := time.ParseDuration(windowStr)
	if err != nil {
		log.Fatalf("bad -window: %v", err)
	}

	// Initialize UI parameters with defaults - refresh is fixed at 10s
	uiParams := &UIParams{
		Window:  window,
		Alpha:   alpha,
		Refresh: 10 * time.Second, // Fixed refresh rate
	}

	// Get the log file path and config using the config system
	cfg, logPath := loadPaths()

	// Create terminal
	t, err := tcell.New()
	if err != nil {
		log.Fatalf("tcell.New => %v", err)
	}
	defer t.Close()

	// Create widgets
	chartWidget, err := createChartWidget()
	if err != nil {
		log.Fatalf("createChartWidget => %v", err)
	}

	textWidget, err := createTextWidget()
	if err != nil {
		log.Fatalf("createTextWidget => %v", err)
	}

	// Data update function (declared here so it can be used in callbacks)
	var updateData func() error

	// Create parameter control widgets with auto-refresh callbacks
	windowInput, alphaInput, err := createInputWidgets(windowStr, alpha, uiParams, &updateData)
	if err != nil {
		log.Fatalf("createInputWidgets => %v", err)
	}

	// Set up the container with layout including controls
	c, err := createUILayout(t, chartWidget, textWidget, windowInput, alphaInput)
	if err != nil {
		log.Fatalf("createUILayout => %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up data refresh and get the update function
	updateData, err = setupDataRefresh(ctx, logPath, uiParams, chartWidget, textWidget, cfg)
	if err != nil {
		log.Fatalf("setupDataRefresh => %v", err)
	}

	// Initial data load
	if err := updateData(); err != nil {
		log.Printf("Initial data load error: %v", err)
	}

	// Create keyboard event handler
	keyboardHandler := createKeyboardHandler(cancel, updateData)

	// Run the dashboard
	_, _, currentRefresh := uiParams.Get()
	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(keyboardHandler), termdash.RedrawInterval(currentRefresh)); err != nil {
		log.Fatalf("termdash.Run => %v", err)
	}
}
