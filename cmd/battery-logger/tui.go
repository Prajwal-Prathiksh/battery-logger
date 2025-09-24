package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/config"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/sysfs"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/widgets"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/text"
)

// formatDurationAuto formats a duration as HH:mm if <24h, or as Xd Yh if >=24h
func formatDurationAuto(dur time.Duration) string {
	if dur < 24*time.Hour {
		h := int(dur.Hours())
		m := int(dur.Minutes()) % 60
		return fmt.Sprintf("%02dh %02dm", h, m)
	} else {
		days := int(dur.Hours()) / 24
		hours := int(dur.Hours()) % 24
		return fmt.Sprintf("%dd %dh", days, hours)
	}
}

// UIParams holds the real-time adjustable parameters
type UIParams struct {
	Refresh time.Duration
	mu      sync.RWMutex
}

// Get returns thread-safe copies of the parameters
func (p *UIParams) Get() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Refresh
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
	LogPath          string
	MaxChargePercent int
	CycleCount       int
	HasCycleCount    bool
}

// createChartWidget creates and configures the time chart widget
func createChartWidget(cfg config.Config) *widgets.BatteryChart {
	return widgets.CreateBatteryChart(
		widgets.YRange(0, 100),
		widgets.YLabel("%"),
		widgets.Title("Battery % Over Time"),
		widgets.DayHours(cfg.DayStartHour, cfg.DayEndHour),
		widgets.DayNightColors(
			cell.ColorNumber(cfg.DayColorNumber),   // Day color from config
			cell.ColorNumber(cfg.NightColorNumber), // Night color from config
		),
	)
}

// createTextWidget creates and configures the text display widget
func createTextWidget() (*text.Text, error) {
	return text.New(text.WrapAtWords())
}

// createUILayout creates the TUI container layout with all widgets
func createUILayout(t terminalapi.Terminal, chartWidget *widgets.BatteryChart, textWidget *text.Text) (*container.Container, error) {
	return container.New(
		t,
		container.Border(linestyle.Light),
		container.BorderTitle("Battery Logger TUI - Tab/Shift+Tab: focus, q: quit, r: refresh"),
		container.KeyFocusNext(keyboard.KeyTab),
		container.KeyFocusPrevious(keyboard.KeyBacktab),
		container.SplitHorizontal(
			container.Top(
				container.ID("chart-container"),
				container.Border(linestyle.Light),
				container.BorderTitle("Battery % Over Time - i/o/mouse wheel: zoom, ←→: pan, esc: reset"),
				container.PlaceWidget(chartWidget),
			),
			container.Bottom(
				container.Border(linestyle.Light),
				container.BorderTitle("Battery Status & Prediction - ↑↓ to scroll"),
				container.PlaceWidget(textWidget),
			),
			container.SplitPercent(70),
		),
	)
}

// processChartData converts battery data to BatteryChart format
func processChartData(rows []analytics.Row) ([]widgets.TimeSeries, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("no data available")
	}

	var series []widgets.TimeSeries
	var chargingPoints []widgets.TimePoint
	var dischargingPoints []widgets.TimePoint

	for _, row := range rows {
		point := widgets.TimePoint{
			Time:  row.T,
			Value: row.Batt,
			State: row.AC,
		}
		if row.AC {
			chargingPoints = append(chargingPoints, point)
		} else {
			dischargingPoints = append(dischargingPoints, point)
		}
	}

	if len(chargingPoints) > 0 {
		series = append(series, widgets.TimeSeries{
			Name:   "Charging",
			Points: chargingPoints,
			Color:  cell.ColorNumber(46), // Bright green for better contrast
		})
	}
	if len(dischargingPoints) > 0 {
		series = append(series, widgets.TimeSeries{
			Name:   "Discharging",
			Points: dischargingPoints,
			Color:  cell.ColorNumber(196), // Bright red for better contrast
		})
	}
	return series, nil
}

// updateChartWidget updates the chart widget with new data
func updateChartWidget(chartWidget *widgets.BatteryChart, series []widgets.TimeSeries) error {
	chartWidget.ClearSeries()
	chartWidget.SetSeries(series)
	return nil
}

// updateChartTitleFromZoom updates the chart title with the current zoom duration
func updateChartTitleFromZoom(c *container.Container, startTime, endTime time.Time) {
	timeDiff := endTime.Sub(startTime)
	span := formatDurationAuto(timeDiff.Round(time.Minute))
	title := fmt.Sprintf("Battery %% Over Time [%s] - i/o/mouse wheel: zoom, ←→: pan, esc: reset", span)
	c.Update("chart-container", container.BorderTitle(title))
}

// generateStatusInfo processes battery data to create status information (logic only)
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
		confidence = conf
		if ok {
			if currentACState {
				rateLabel = "Charge Rate"
			} else {
				rateLabel = "Discharge Rate"
			}
			dur := time.Duration(estimateMins * float64(time.Minute)).Round(time.Minute)
			est = formatDurationAuto(dur)
			slopeStr = fmt.Sprintf("%.3f %%/min", rate)
		} else {
			if currentACState {
				rateLabel = "Charge Rate"
			} else {
				rateLabel = "Discharge Rate"
			}
			est = "—"
			slopeStr = "n/a"
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
	startTime := rows[0].T.Format("Jan 2 15:04")
	endTime := rows[len(rows)-1].T.Format("Jan 2 15:04")

	// Get config file paths
	_, existingConfigPaths := config.GetConfigPaths()
	var configStr string
	if len(existingConfigPaths) == 0 {
		configStr = "  Config: Using defaults (no config file found)" // nf-md-cog
	} else if len(existingConfigPaths) == 1 {
		configStr = fmt.Sprintf("  Config file: %s", existingConfigPaths[0]) // nf-md-cog
	} else {
		configStr = fmt.Sprintf("  Config files: %s (+ %d more)", existingConfigPaths[len(existingConfigPaths)-1], len(existingConfigPaths)-1) // nf-md-cog
	}

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
		LogPath:          logPath,
		MaxChargePercent: cfg.MaxChargePercent,
		CycleCount:       cycleCount,
		HasCycleCount:    hasCycleCount,
	}
}

// Formatting section for status text display
type lineSpec struct {
	Text     string
	Color    cell.Color
	UseColor bool
}

// buildStatusLines centralizes ALL string construction & styling.
func buildStatusLines(info StatusInfo) []lineSpec {
	var lines []lineSpec

	appendLine := func(txt string, color cell.Color, useColor bool) {
		// Use strings package meaningfully to normalize whitespace.
		txt = strings.TrimSpace(txt)
		lines = append(lines, lineSpec{Text: txt, Color: color, UseColor: useColor})
	}

	// Header: AC status
	acStatus := "Unplugged"
	acIcon := "󱐤"
	if info.Latest.AC {
		acStatus = "Plugged In"
		acIcon = ""
	}
	appendLine(fmt.Sprintf("%s  AC Status: %s", acIcon, acStatus), cell.ColorYellow, true)

	// Delta since last transition
	if !info.TransitionTime.IsZero() {
		durationSince := info.Latest.T.Sub(info.TransitionTime).Round(time.Minute)
		if info.Latest.AC {
			battGain := info.Latest.Batt - info.TransitionBatt
			appendLine(
				fmt.Sprintf("--    Plugged in for %s, battery ↑ %.1f%% (start: %.1f%%)",
					formatDurationAuto(durationSince), battGain, info.TransitionBatt),
				0, false,
			)
		} else {
			battDrop := info.TransitionBatt - info.Latest.Batt
			appendLine(
				fmt.Sprintf("--    On battery for %s (since: %s), battery ↓ %.1f%% (start: %.1f%%)",
					formatDurationAuto(durationSince), info.TransitionTime.Format("Jan 2 15:04"), battDrop, info.TransitionBatt),
				0, false,
			)
		}
	}
	if info.Latest.AC {
		appendLine(fmt.Sprintf("--    Time to Full (%d%%): %s", info.MaxChargePercent, info.Estimate), 0, false)
	} else {
		appendLine(fmt.Sprintf("--    Time to Empty (0%%): %s", info.Estimate), 0, false)
	}

	// Spacer
	appendLine("", 0, false)

	// Battery status section
	appendLine("󰤁  Battery Status:", 0, false)
	// Current battery & cycles
	appendLine(fmt.Sprintf("--    Current Battery: %.1f%%", info.Latest.Batt), 0, false)
	if info.HasCycleCount {
		appendLine(fmt.Sprintf("--    Battery Cycles: %d", info.CycleCount), 0, false)
	}

	// Rate + estimate
	appendLine(fmt.Sprintf("--    %s: %s %s", info.RateLabel, info.SlopeStr, info.Confidence), 0, false)

	// Spacer
	appendLine("", 0, false)

	// Summary section
	appendLine("  Data Summary:", 0, false)
	appendLine(fmt.Sprintf("--    Total samples: %d (spanning %s)", info.TotalSamples, formatDurationAuto(info.TimeRange.Round(time.Minute))), 0, false)
	appendLine(fmt.Sprintf("--    AC plugged: %d samples", info.ACSamples), cell.ColorGreen, true)
	appendLine(fmt.Sprintf("--    On battery: %d samples", info.BattSamples), cell.ColorRed, true)
	appendLine(fmt.Sprintf("--    Time range: %s to %s", info.StartTime, info.EndTime), 0, false)

	// Spacer
	appendLine("", 0, false)

	// Paths & config
	appendLine(fmt.Sprintf("  Data file: %s", info.LogPath), 0, false)
	appendLine(info.ConfigStr, 0, false)

	return lines
}

// updateStatusText writes formatted status information to the text widget
func updateStatusText(textWidget *text.Text, info StatusInfo) {
	textWidget.Reset()
	for _, ln := range buildStatusLines(info) {
		if ln.UseColor {
			textWidget.Write(ln.Text+"\n", text.WriteCellOpts(cell.FgColor(ln.Color)))
		} else {
			textWidget.Write(ln.Text + "\n")
		}
	}
}

// setupDataRefresh sets up periodic data refresh and returns the update function
func setupDataRefresh(ctx context.Context, logPath string, uiParams *UIParams, chartWidget *widgets.BatteryChart, textWidget *text.Text, cfg config.Config, c *container.Container, alpha float64) (func() error, error) {
	updateData := func() error {
		rows, err := readCSV(logPath)
		if err != nil || len(rows) == 0 {
			textWidget.Write(fmt.Sprintf("Could not read data from %s: %v\n", logPath, err), text.WriteCellOpts(cell.FgColor(cell.ColorRed)))
			textWidget.Write("Press q to quit, r to refresh\n")
			return nil
		}

		if len(rows) == 0 {
			textWidget.Write("No data available.\n", text.WriteCellOpts(cell.FgColor(cell.ColorYellow)))
			textWidget.Write("Press q to quit, r to refresh\n")
			return nil
		}

		// Process chart data
		series, err := processChartData(rows)
		if err != nil {
			return fmt.Errorf("processing chart data: %v", err)
		}

		// Update chart
		if err := updateChartWidget(chartWidget, series); err != nil {
			return fmt.Errorf("updating chart: %v", err)
		}

		// Update chart title with current zoom window (not full data range)
		startTime, endTime, _ := chartWidget.GetCurrentWindow()
		updateChartTitleFromZoom(c, startTime, endTime)

		// Generate and update status text
		statusInfo := generateStatusInfo(rows, alpha, uiParams, logPath, cfg)
		updateStatusText(textWidget, statusInfo)

		return nil
	}

	// Set up periodic refresh
	currentRefresh := uiParams.Get()
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
	var alpha float64

	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	fs.Float64Var(&alpha, "alpha", 0.05, "exponential decay per minute for weights (e.g., 0.05)")

	if len(os.Args) > 2 {
		fs.Parse(os.Args[2:])
	}

	// Initialize UI parameters with defaults - refresh is fixed at 10s
	uiParams := &UIParams{
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
	chartWidget := createChartWidget(cfg)

	textWidget, err := createTextWidget()
	if err != nil {
		log.Fatalf("createTextWidget => %v", err)
	}

	// Data update function (declared here so it can be used in callbacks)
	var updateData func() error

	// Set up the container with layout
	c, err := createUILayout(t, chartWidget, textWidget)
	if err != nil {
		log.Fatalf("createUILayout => %v", err)
	}

	// Set up zoom change callback to update chart title dynamically
	chartWidget.SetOnZoomChange(func(startTime, endTime time.Time, duration time.Duration) {
		updateChartTitleFromZoom(c, startTime, endTime)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up data refresh and get the update function
	updateData, err = setupDataRefresh(ctx, logPath, uiParams, chartWidget, textWidget, cfg, c, alpha)
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
	currentRefresh := uiParams.Get()
	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(keyboardHandler), termdash.RedrawInterval(currentRefresh)); err != nil {
		log.Fatalf("termdash.Run => %v", err)
	}
}
