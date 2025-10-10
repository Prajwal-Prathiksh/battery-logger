package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/Prajwal-Prathiksh/battery-zen/internal/analytics"
	"github.com/Prajwal-Prathiksh/battery-zen/internal/config"
	"github.com/Prajwal-Prathiksh/battery-zen/internal/lock"
	"github.com/Prajwal-Prathiksh/battery-zen/internal/logfile"
	"github.com/Prajwal-Prathiksh/battery-zen/internal/sysfs"
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
	fmt.Fprintf(os.Stderr, `battery-zen commands:
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
	lockPath := cfg.LogDir + "/.battery-zen.pid"
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

// tuiCmd implements the TUI command using termdash with real-time parameter controls
func tuiCmd() {
	runTUI()
}
