package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/config"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/lock"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/logfile"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/sysfs"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/tcell"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/linechart"
	"github.com/mum4k/termdash/widgets/text"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) == 1 {
		usage()
		return
	}

	switch os.Args[1] {
	case "sample":
		sampleCmd()
	case "run":
		runCmd()
	case "trim":
		trimCmd()
	case "status":
		statusCmd()
	case "tui":
		tuiCmd()
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `battery-logger commands:
  sample     Append one CSV sample (used by systemd timer)
  run        Daemon loop (periodic)
  trim       Force trim to max_lines
  status     Print current reading and path
  tui        Launch interactive TUI for data visualization
`)
	os.Exit(2)
}

func loadPaths() (config.Config, string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logPath, err := config.XDGLogPath(cfg)
	if err != nil {
		log.Fatalf("paths: %v", err)
	}
	if err := logfile.EnsureDir(logPath); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	return cfg, logPath
}

func sampleOnce(cfg config.Config, logPath string) error {
	w := &logfile.Writer{Path: logPath}
	ac := sysfs.ACOnline()
	pct, ok := sysfs.BatteryPercent()
	if !ok {
		return fmt.Errorf("battery percent not found")
	}
	ts := config.Now(cfg).Format(time.RFC3339)
	if err := w.AppendCSV(ts, ac, pct); err != nil {
		return err
	}
	// Trim if we exceeded threshold
	lines, err := w.LineCount()
	if err == nil && lines > (cfg.MaxLines+cfg.TrimBuffer+1) { // +1 header
		if err := w.TrimToLast(cfg.MaxLines); err != nil {
			return err
		}
	}
	return nil
}

func sampleCmd() {
	cfg, logPath := loadPaths()
	if err := sampleOnce(cfg, logPath); err != nil {
		log.Fatalf("sample: %v", err)
	}
}

func runCmd() {
	cfg, logPath := loadPaths()
	// Guard with pidfile so only one daemon runs
	lockPath := cfg.LogDir + "/.battery-logger.pid"
	pf := &lock.PIDFile{Path: lockPath}
	ok, err := pf.Acquire()
	if err != nil {
		log.Fatalf("lock: %v", err)
	}
	if !ok {
		log.Fatalf("another instance is running")
	}
	defer pf.Release()

	interval := time.Duration(cfg.IntervalSecs) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial tick immediately
	if err := sampleOnce(cfg, logPath); err != nil {
		log.Printf("sample: %v", err)
	}

	for range ticker.C {
		if err := sampleOnce(cfg, logPath); err != nil {
			log.Printf("sample: %v", err)
		}
	}
}

func trimCmd() {
	cfg, logPath := loadPaths()
	w := &logfile.Writer{Path: logPath}
	if err := w.TrimToLast(cfg.MaxLines); err != nil {
		log.Fatalf("trim: %v", err)
	}
}

func statusCmd() {
	cfg, logPath := loadPaths()
	ac := sysfs.ACOnline()
	pct, _ := sysfs.BatteryPercent()
	fmt.Printf("ac_connected=%t battery_life=%d ts=%s file=%s\n",
		ac, pct, config.Now(cfg).Format(time.RFC3339), logPath)
}

// optional flags example (not strictly needed):
func init() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		_ = fs // add overrides if you wish
	}
}

// readCSV reads the battery CSV file and parses it into Row structs
func readCSV(path string) ([]analytics.Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	return analytics.ParseCSVRows(rows)
}

// findLastACTransition finds the most recent AC status change and returns
// the time and battery percentage when the current AC status started.
// Returns zero time and 0.0 battery if no transition found.
func findLastACTransition(rows []analytics.Row) (time.Time, float64) {
	if len(rows) == 0 {
		return time.Time{}, 0.0
	}

	currentACStatus := rows[len(rows)-1].AC

	// Walk backwards from the end to find the last status change
	for i := len(rows) - 2; i >= 0; i-- {
		if rows[i].AC != currentACStatus {
			// Found the transition point - return the time and battery of the first sample with current status
			if i+1 < len(rows) {
				return rows[i+1].T, rows[i+1].Batt
			}
		}
	}

	// No transition found in the data, current status has been the same throughout
	// Return the time and battery of the first sample
	return rows[0].T, rows[0].Batt
}

// BinDataPoint represents a time bin with battery data
type BinDataPoint struct {
	Time    time.Time
	Batt    float64
	AC      bool
	HasData bool
}

// binDataToTimeGrid bins the data into time intervals for plotting
func binDataToTimeGrid(rows []analytics.Row, binSize time.Duration, window time.Duration) []BinDataPoint {
	if len(rows) == 0 {
		return nil
	}

	// Calculate the time range
	endTime := rows[len(rows)-1].T
	startTime := endTime.Add(-window)

	// Create bins
	var bins []BinDataPoint
	for t := startTime; t.Before(endTime) || t.Equal(endTime); t = t.Add(binSize) {
		bins = append(bins, BinDataPoint{
			Time:    t,
			HasData: false,
		})
	}

	// Fill bins with data
	for _, row := range rows {
		if row.T.Before(startTime) {
			continue
		}

		// Find the appropriate bin
		binIndex := int(row.T.Sub(startTime) / binSize)
		if binIndex >= 0 && binIndex < len(bins) {
			bins[binIndex].Batt = row.Batt
			bins[binIndex].AC = row.AC
			bins[binIndex].HasData = true
		}
	}

	return bins
}

// createTimeBasedSeries creates series data with intelligent time-based x-labels for zooming
func createTimeBasedSeries(bins []BinDataPoint, startTime time.Time) ([]float64, []float64, map[int]string) {
	acSeries := make([]float64, len(bins))
	battSeries := make([]float64, len(bins))

	// Create intelligent time labels based on data density
	labels := make(map[int]string)

	for i, bin := range bins {
		if bin.HasData {
			if bin.AC {
				acSeries[i] = bin.Batt
				battSeries[i] = math.NaN()
			} else {
				battSeries[i] = bin.Batt
				acSeries[i] = math.NaN()
			}
		} else {
			acSeries[i] = math.NaN()
			battSeries[i] = math.NaN()
		}
	}

	// Create time labels for all points - termdash will intelligently display them
	// We'll provide labels at different granularities for different zoom levels
	for i, bin := range bins {
		minute := bin.Time.Minute()

		// Always provide a time label - format depends on the time
		if minute == 0 {
			// Top of the hour - show HH:00
			labels[i] = bin.Time.Format("15:04")
		} else if minute%15 == 0 {
			// Quarter hours - show HH:15, HH:30, HH:45
			labels[i] = bin.Time.Format("15:04")
		} else if minute%5 == 0 {
			// Every 5 minutes - useful for zoomed views
			labels[i] = bin.Time.Format("15:04")
		} else {
			// For very zoomed views, show all times
			labels[i] = bin.Time.Format("15:04")
		}
	}

	return acSeries, battSeries, labels
}

// tuiCmd implements the TUI command using termdash
func tuiCmd() {
	var windowStr string
	var alpha float64
	var refreshStr string

	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	fs.StringVar(&windowStr, "window", "6h", "rolling window to display & regress (e.g., 10m, 30m, 2h)")
	fs.Float64Var(&alpha, "alpha", 0.05, "exponential decay per minute for weights (e.g., 0.05)")
	fs.StringVar(&refreshStr, "refresh", "10s", "UI refresh period (e.g., 2s, 1s, 5s)")

	if len(os.Args) > 2 {
		fs.Parse(os.Args[2:])
	}

	window, err := time.ParseDuration(windowStr)
	if err != nil {
		log.Fatalf("bad -window: %v", err)
	}
	refresh, err := time.ParseDuration(refreshStr)
	if err != nil {
		log.Fatalf("bad -refresh: %v", err)
	}

	// Get the log file path using the config system
	_, logPath := loadPaths()

	// Create terminal
	t, err := tcell.New()
	if err != nil {
		log.Fatalf("tcell.New => %v", err)
	}
	defer t.Close()

	// Create widgets
	chartWidget, err := linechart.New(
		linechart.AxesCellOpts(cell.FgColor(cell.ColorWhite)),
		linechart.YLabelCellOpts(cell.FgColor(cell.ColorWhite)),
		linechart.XLabelCellOpts(cell.FgColor(cell.ColorWhite)),
	)
	if err != nil {
		log.Fatalf("linechart.New => %v", err)
	}

	textWidget, err := text.New(text.RollContent(), text.WrapAtWords())
	if err != nil {
		log.Fatalf("text.New => %v", err)
	}

	// Set up the container with layout
	c, err := container.New(
		t,
		container.Border(linestyle.Light),
		container.BorderTitle("Battery Logger TUI - Press q to quit, r to refresh"),
		container.SplitHorizontal(
			container.Top(
				container.Border(linestyle.Light),
				container.BorderTitle("Battery % Over Time"),
				container.PlaceWidget(chartWidget),
			),
			container.Bottom(
				container.Border(linestyle.Light),
				container.BorderTitle("Battery Status & Prediction"),
				container.PlaceWidget(textWidget),
			),
			container.SplitPercent(70),
		),
	)
	if err != nil {
		log.Fatalf("container.New => %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Data update function
	updateData := func() error {
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

		// Create time-based bins (1-minute bins for finest granularity)
		binSize := 1 * time.Minute
		bins := binDataToTimeGrid(rows, binSize, window)
		dataStartTime := rows[0].T
		acSeries, battSeries, labels := createTimeBasedSeries(bins, dataStartTime)

		// Clear previous chart data
		chartWidget.Series("ac", nil)
		chartWidget.Series("battery", nil)

		// Add series with gaps (NaN values create gaps)
		var hasAC, hasBatt bool
		acValues := []float64{}
		battValues := []float64{}

		for i := range acSeries {
			if !math.IsNaN(acSeries[i]) {
				hasAC = true
			}
			if !math.IsNaN(battSeries[i]) {
				hasBatt = true
			}
			acValues = append(acValues, acSeries[i])
			battValues = append(battValues, battSeries[i])
		}

		// Set Y-axis range - remove invalid API call

		// Set custom X-axis labels for time - these will be used by termdash for zooming
		xLabels := labels

		// Add series to chart with time-based labels
		if hasAC {
			if err := chartWidget.Series("ac", acValues,
				linechart.SeriesCellOpts(cell.FgColor(cell.ColorGreen)),
				linechart.SeriesXLabels(xLabels),
			); err != nil {
				return fmt.Errorf("setting AC series: %v", err)
			}
		}

		if hasBatt {
			if err := chartWidget.Series("battery", battValues,
				linechart.SeriesCellOpts(cell.FgColor(cell.ColorRed)),
				linechart.SeriesXLabels(xLabels),
			); err != nil {
				return fmt.Errorf("setting battery series: %v", err)
			}
		}

		// Update status text
		textWidget.Reset()

		// Latest status
		latest := rows[len(rows)-1]
		acStatus := "Unplugged"
		acIcon := "🔋"
		if latest.AC {
			acStatus = "Plugged In"
			acIcon = "🔌"
		}

		// Find when the current AC status started
		transitionTime, transitionBatt := findLastACTransition(rows)
		var sinceStr string
		if !transitionTime.IsZero() {
			sinceStr = fmt.Sprintf(" (Since: %s, %.1f%%)", transitionTime.Format("15:04"), transitionBatt)
		}

		// For regression, consider only the most recent contiguous unplugged points
		var unplugged []analytics.Row
		for i := len(rows) - 1; i >= 0; i-- {
			if !rows[i].AC {
				unplugged = append([]analytics.Row{rows[i]}, unplugged...)
			} else {
				break
			}
		}

		var est string
		var slopeStr string
		var confidence string

		if len(unplugged) >= 2 {
			b, _, ok := analytics.WeightedLinReg(unplugged, alpha)
			if ok && b < -1e-6 {
				mins := -latest.Batt / b
				est = analytics.FmtDur(mins)
				confidence = fmt.Sprintf("(based on %d unplugged samples)", len(unplugged))
			} else if ok && b >= -1e-6 {
				est = "∞ (not discharging or charging)"
				confidence = ""
			} else {
				est = "—"
				confidence = "(regression failed)"
			}
			slopeStr = fmt.Sprintf("%.3f %%/min", b)
		} else {
			est = "—"
			slopeStr = "n/a"
			confidence = "(need ≥2 unplugged samples)"
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

		// Write status information
		statusLines := []string{
			fmt.Sprintf("%s AC Status: %s%s", acIcon, acStatus, sinceStr),
			fmt.Sprintf("🔋 Current Battery: %.1f%%", latest.Batt),
			fmt.Sprintf("📈 Discharge Rate: %s", slopeStr),
			fmt.Sprintf("⏱️  Time to 0%%: %s %s", est, confidence),
			"",
			fmt.Sprintf("📊 Data Summary (window: %s):", window),
			fmt.Sprintf("   Total samples: %d (spanning %s)", totalSamples, timeRange.Round(time.Minute).String()),
			fmt.Sprintf("   AC plugged: %d samples", acSamples),
			fmt.Sprintf("   On battery: %d samples", battSamples),
			fmt.Sprintf("   Time range: %s to %s", startTime, endTime),
			"",
			fmt.Sprintf("⚙️  Settings: Alpha=%.3f, Refresh=%s", alpha, refresh),
			fmt.Sprintf("📄 Data file: %s", logPath),
			configStr,
			"",
			"📝 Note: 1) X-axis shows time (HH:MM), use mouse/keys to zoom",
			"         2) Green line = AC plugged, Red line = On battery",
			"         3) Gaps indicate missing data periods",
			"Press q to quit, r to refresh now, ↑↓ to scroll",
		}

		for _, line := range statusLines {
			var opts []text.WriteOption
			if strings.Contains(line, "AC plugged") {
				opts = append(opts, text.WriteCellOpts(cell.FgColor(cell.ColorGreen)))
			} else if strings.Contains(line, "On battery") {
				opts = append(opts, text.WriteCellOpts(cell.FgColor(cell.ColorRed)))
			} else if strings.HasPrefix(line, "🔋") || strings.HasPrefix(line, "🔌") {
				opts = append(opts, text.WriteCellOpts(cell.FgColor(cell.ColorYellow)))
			}

			textWidget.Write(line+"\n", opts...)
		}

		return nil
	}

	// Initial data load
	if err := updateData(); err != nil {
		log.Printf("Initial data load error: %v", err)
	}

	// Set up periodic refresh
	go func() {
		ticker := time.NewTicker(refresh)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := updateData(); err != nil {
					log.Printf("Data update error: %v", err)
				}
			}
		}
	}()

	// Handle keyboard events
	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == 'q' || k.Key == 'Q' {
			cancel()
		}
		if k.Key == 'r' || k.Key == 'R' {
			if err := updateData(); err != nil {
				log.Printf("Manual refresh error: %v", err)
			}
		}
	}

	// Run the dashboard
	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(quitter), termdash.RedrawInterval(refresh)); err != nil {
		log.Fatalf("termdash.Run => %v", err)
	}
}
