package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

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

// Row represents a single CSV record
type Row struct {
	T    time.Time
	AC   bool
	Batt float64
}

// parseBoolLoose parses boolean values in various formats
func parseBoolLoose(s string) (bool, error) {
	ss := strings.TrimSpace(strings.ToLower(s))
	switch ss {
	case "true", "t", "1", "yes", "y":
		return true, nil
	case "false", "f", "0", "no", "n":
		return false, nil
	default:
		// Try parsing as integer (0 = false, anything else = true)
		if val, err := strconv.Atoi(ss); err == nil {
			return val != 0, nil
		}
		return false, fmt.Errorf("bad bool: %q", s)
	}
}

// readCSV reads the battery CSV file and parses it into Row structs
func readCSV(path string) ([]Row, error) {
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
	if len(rows) == 0 {
		return nil, errors.New("empty csv")
	}

	// Find columns by header name (flexible: case-insensitive, allow spaces)
	col := func(name string) int {
		name = strings.ToLower(strings.TrimSpace(name))
		for i, h := range rows[0] {
			if strings.ToLower(strings.TrimSpace(h)) == name {
				return i
			}
		}
		return -1
	}

	tsIdx := col("timestamp")
	acIdx := col("ac_connected")
	if acIdx == -1 { acIdx = col("ac") }
	if acIdx == -1 { acIdx = col("ac plugged in (bool)") }
	if acIdx == -1 { acIdx = col("ac plugged in") }
	battIdx := col("battery_life")
	if battIdx == -1 { battIdx = col("battery") }
	if battIdx == -1 { battIdx = col("battery life (%)") }
	
	if tsIdx == -1 || acIdx == -1 || battIdx == -1 {
		return nil, fmt.Errorf("expected headers: timestamp, ac_connected, battery_life (or similar)")
	}

	var out []Row
	for i := 1; i < len(rows); i++ {
		rec := rows[i]
		if len(rec) <= battIdx || len(rec) <= tsIdx || len(rec) <= acIdx {
			continue
		}
		
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(rec[tsIdx]))
		if err != nil {
			// Try some common fallback formats if needed
			layouts := []string{
				"2006-01-02 15:04:05",
				"2006-01-02 15:04:05 -0700",
				"2006-01-02T15:04:05",
			}
			ok := false
			for _, lay := range layouts {
				if tt, e2 := time.Parse(lay, strings.TrimSpace(rec[tsIdx])); e2 == nil {
					t = tt
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		
		ac, err := parseBoolLoose(rec[acIdx])
		if err != nil {
			continue
		}
		
		b, err := strconv.ParseFloat(strings.TrimSpace(rec[battIdx]), 64)
		if err != nil {
			continue
		}
		
		out = append(out, Row{T: t, AC: ac, Batt: b})
	}
	return out, nil
}

// filterWindow filters rows to only include those within the specified time window
func filterWindow(rows []Row, since time.Duration) []Row {
	if len(rows) == 0 {
		return nil
	}
	cut := rows[len(rows)-1].T.Add(-since)
	i := 0
	for i < len(rows) && rows[i].T.Before(cut) {
		i++
	}
	return rows[i:]
}

// weightedLinReg performs weighted linear regression y = a + b*x
// x in minutes relative to last point (<=0), weights w = exp(alpha*x)
// returns slope b (% per minute), intercept a (% at x=0 i.e., "now"), ok
func weightedLinReg(rows []Row, alpha float64) (float64, float64, bool) {
	if len(rows) < 2 {
		return 0, 0, false
	}
	tNow := rows[len(rows)-1].T

	var sumW, sumWX, sumWY, sumWXX, sumWXY float64
	for _, r := range rows {
		x := r.T.Sub(tNow).Minutes() // <= 0
		w := math.Exp(alpha * x)     // more recent -> larger weight
		y := r.Batt
		sumW += w
		sumWX += w * x
		sumWY += w * y
		sumWXX += w * x * x
		sumWXY += w * x * y
	}

	den := sumW*sumWXX - sumWX*sumWX
	if den == 0 {
		return 0, 0, false
	}
	b := (sumW*sumWXY - sumWX*sumWY) / den
	a := (sumWY - b*sumWX) / sumW
	return b, a, true
}

// fmtDur formats duration in minutes to a human-readable string
func fmtDur(mins float64) string {
	if math.IsNaN(mins) || math.IsInf(mins, 0) || mins < 0 {
		return "—"
	}
	d := time.Duration(mins * float64(time.Minute))
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	return fmt.Sprintf("%dh %dm", h, m)
}

// tuiCmd implements the TUI command
func tuiCmd() {
	var windowStr string
	var alpha float64
	var refreshStr string

	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	fs.StringVar(&windowStr, "window", "2h", "rolling window to display & regress (e.g., 10m, 30m, 2h)")
	fs.Float64Var(&alpha, "alpha", 0.05, "exponential decay per minute for weights (e.g., 0.05)")
	fs.StringVar(&refreshStr, "refresh", "5s", "UI refresh period (e.g., 2s, 1s, 5s)")
	
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

	// Create info widget
	info := widgets.NewParagraph()
	info.Title = "Battery Status & Prediction"
	info.SetRect(0, 20, 100, 30)
	info.BorderStyle.Fg = ui.ColorYellow

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
			info.Text = fmt.Sprintf("Could not read data from %s: %v\nPress q to quit.", logPath, err)
			ui.Render(grid)
			return
		}
		
		rows = filterWindow(rows, window)
		if len(rows) == 0 {
			info.Text = "No recent data in window.\nPress q to quit."
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
		acColor := "🔴"
		if latest.AC {
			acStatus = "Plugged In"
			acColor = "🟢"
		}

		// For regression, consider only unplugged points in window
		var unplugged []Row
		for _, r := range rows {
			if !r.AC {
				unplugged = append(unplugged, r)
			}
		}

		var est string
		var slopeStr string
		var confidence string
		
		if len(unplugged) >= 2 {
			b, a, ok := weightedLinReg(unplugged, alpha)
			// b is % per minute (negative when discharging), a is % now
			if ok && b < -1e-6 {
				mins := -a / b
				est = fmtDur(mins)
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
		
		info.Text = fmt.Sprintf(
			"%s AC Status: %s\n"+
			"🔋 Current Battery: %.1f%%\n"+
			"📈 Discharge Rate: %s\n"+
			"⏱️  Time to 0%%: %s %s\n"+
			"\n"+
			"📊 Data Summary (window: %s):\n"+
			"   Total samples: %d (spanning %s)\n"+
			"   AC plugged: %d samples 🟢\n"+
			"   On battery: %d samples 🔴\n"+
			"   Time range: %s to %s\n"+
			"\n"+
			"⚙️  Settings: Alpha=%.3f, Refresh=%s\n"+
			"📄 Data file: %s\n"+
			"\n"+
			"📝 Note: Chart x-axis shows sample sequence, time span in title\n"+
			"Press q to quit, r to refresh now",
			acColor, acStatus,
			latest.Batt,
			slopeStr,
			est, confidence,
			window,
			totalSamples, timeRange.Round(time.Minute).String(),
			acSamples,
			battSamples,
			startTime, endTime,
			alpha, refresh,
			logPath,
		)

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
