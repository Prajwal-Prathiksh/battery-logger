package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"battery-logger/internal/analytics"
	"battery-logger/internal/config"
	"battery-logger/internal/lock"
	"battery-logger/internal/logfile"
	"battery-logger/internal/sysfs"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
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

// tuiCmd implements the TUI command
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

	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	// Create plot widget
	plot := widgets.NewPlot()
	plot.Title = "Battery % Over Time"
	plot.PlotType = widgets.LineChart
	plot.Marker = widgets.MarkerDot
	plot.Data = [][]float64{{}}
	plot.SetRect(0, 0, 100, 20)
	plot.AxesColor = ui.ColorWhite
	plot.LineColors = []ui.Color{ui.ColorGreen, ui.ColorRed} // Green for AC, Red for battery

	// Create info widget with scrolling capability
	info := widgets.NewList()
	info.Title = "Battery Status & Prediction (↑↓ to scroll)"
	info.SetRect(0, 20, 100, 30)
	info.BorderStyle.Fg = ui.ColorYellow
	info.SelectedRowStyle = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierClear)
	info.WrapText = false

	// Create grid layout
	grid := ui.NewGrid()
	termW, termH := ui.TerminalDimensions()
	grid.SetRect(0, 0, termW, termH)
	grid.Set(
		ui.NewRow(0.7, plot),
		ui.NewRow(0.3, info),
	)

	ui.Render(grid)

	ticker := time.NewTicker(refresh)
	defer ticker.Stop()

	draw := func() {
		rows, err := readCSV(logPath)
		if err != nil || len(rows) == 0 {
			info.Rows = []string{fmt.Sprintf("Could not read data from %s: %v", logPath, err), "Press q to quit."}
			ui.Render(grid)
			return
		}
		
		rows = analytics.FilterWindow(rows, window)
		if len(rows) == 0 {
			info.Rows = []string{"No recent data in window.", "Press q to quit."}
			ui.Render(grid)
			return
		}

		// Create chronological series with NaN for gaps to maintain time positioning
		acSeries := make([]float64, len(rows))
		battSeries := make([]float64, len(rows))
		hasAC := false
		hasBatt := false
		
		for i, r := range rows {
			if r.AC {
				acSeries[i] = r.Batt
				battSeries[i] = math.NaN()
				hasAC = true
			} else {
				battSeries[i] = r.Batt
				acSeries[i] = math.NaN()
				hasBatt = true
			}
		}

		// Update plot data - show both series if both exist
		if hasAC && hasBatt {
			plot.Data = [][]float64{acSeries, battSeries}
			plot.LineColors = []ui.Color{ui.ColorGreen, ui.ColorRed}
		} else if hasAC {
			plot.Data = [][]float64{acSeries}
			plot.LineColors = []ui.Color{ui.ColorGreen}
		} else if hasBatt {
			plot.Data = [][]float64{battSeries}
			plot.LineColors = []ui.Color{ui.ColorRed}
		} else {
			// Fallback to showing all data as one series
			series := make([]float64, len(rows))
			for i, r := range rows {
				series[i] = r.Batt
			}
			plot.Data = [][]float64{series}
			plot.LineColors = []ui.Color{ui.ColorCyan}
		}

		// Calculate time span and update title with time context
		timeSpan := rows[len(rows)-1].T.Sub(rows[0].T)
		startTime := rows[0].T.Format("15:04")
		endTime := rows[len(rows)-1].T.Format("15:04")
		
		plot.Title = fmt.Sprintf("Battery %% Over Time (%s - %s, %s)", startTime, endTime, timeSpan.Round(time.Minute).String())

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
        // Start from the end and work backwards to find the most recent contiguous unplugged batch
        for i := len(rows) - 1; i >= 0; i-- {
            if !rows[i].AC {
                // Prepend to maintain chronological order
                unplugged = append([]analytics.Row{rows[i]}, unplugged...)
            } else {
                // Hit a plugged point, stop collecting
                break
            }
        }

		var est string
		var slopeStr string
		var confidence string
		
		if len(unplugged) >= 2 {
			b, _, ok := analytics.WeightedLinReg(unplugged, alpha)
			// b is % per minute (negative when discharging)
			// Use actual latest battery level, not regression intercept
			if ok && b < -1e-6 {
				mins := -latest.Batt / b

				// Estimate time to 0%
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
		
		info.Rows = []string{
			fmt.Sprintf("%s AC Status: %s%s", acIcon, acStatus, sinceStr),
			fmt.Sprintf("🔋 Current Battery: %.1f%%", latest.Batt),
			fmt.Sprintf("📈 Discharge Rate: %s", slopeStr),
			fmt.Sprintf("⏱️  Time to 0%%: %s %s", est, confidence),
			"",
			fmt.Sprintf("📊 Data Summary (window: %s):", window),
			fmt.Sprintf("   Total samples: %d (spanning %s)", totalSamples, timeRange.Round(time.Minute).String()),
			fmt.Sprintf("   AC plugged: %d samples [🟢](fg:green)", acSamples),
			fmt.Sprintf("   On battery: %d samples [🔴](fg:red)", battSamples),
			fmt.Sprintf("   Time range: %s to %s", startTime, endTime),
			"",
			fmt.Sprintf("⚙️  Settings: Alpha=%.3f, Refresh=%s", alpha, refresh),
			fmt.Sprintf("📄 Data file: %s", logPath),
			configStr,
			"",
			"📝 Note: 1) Chart x-axis shows sample sequence, time span in title",
			"         2) Green line = AC plugged, Red line = On battery",
			"Press q to quit, r to refresh now, ↑↓ to scroll",
		}

		// Resize-aware
		termW, termH = ui.TerminalDimensions()
		grid.SetRect(0, 0, termW, termH)
		ui.Render(grid)
	}

	// Initial draw
	draw()

	// Event loop
	uiEvents := ui.PollEvents()
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "r":
				draw()
			case "<Up>", "k":
				info.ScrollUp()
				ui.Render(grid)
			case "<Down>", "j":
				info.ScrollDown()
				ui.Render(grid)
			case "<PageUp>":
				info.ScrollPageUp()
				ui.Render(grid)
			case "<PageDown>":
				info.ScrollPageDown()
				ui.Render(grid)
			case "<Home>":
				info.ScrollTop()
				ui.Render(grid)
			case "<End>":
				info.ScrollBottom()
				ui.Render(grid)
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				grid.SetRect(0, 0, payload.Width, payload.Height)
				ui.Render(grid)
			}
		case <-ticker.C:
			draw()
		}
	}
}
