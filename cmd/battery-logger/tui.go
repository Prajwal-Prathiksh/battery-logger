package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/tui"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/terminal/tcell"
)

// runTUI implements the TUI command using termdash with real-time parameter controls
func runTUI() {
	var alpha float64

	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	fs.Float64Var(&alpha, "alpha", 0.05, "exponential decay per minute for weights (e.g., 0.05)")

	if len(os.Args) > 2 {
		fs.Parse(os.Args[2:])
	}

	// Initialize UI parameters with defaults - refresh is fixed at 10s
	uiParams := &tui.UIParams{
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
	chartWidget := tui.CreateChartWidget(cfg)

	textWidget, err := tui.CreateTextWidget()
	if err != nil {
		log.Fatalf("CreateTextWidget => %v", err)
	}

	sotBarChart, err := tui.CreateSOTBarChart()
	if err != nil {
		log.Fatalf("CreateSOTBarChart => %v", err)
	}

	// Data update function (declared here so it can be used in callbacks)
	var updateData func() error

	// Set up the container with layout
	c, err := tui.CreateUILayout(t, chartWidget, textWidget, sotBarChart)
	if err != nil {
		log.Fatalf("CreateUILayout => %v", err)
	}

	// Set up zoom change callback to update chart title dynamically
	chartWidget.SetOnZoomChange(func(startTime, endTime time.Time, duration time.Duration) {
		tui.UpdateChartTitleFromZoom(c, startTime, endTime)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up data refresh and get the update function
	updateData, err = tui.SetupDataRefresh(ctx, logPath, uiParams, chartWidget, textWidget, sotBarChart, cfg, c, alpha, readCSV)
	if err != nil {
		log.Fatalf("SetupDataRefresh => %v", err)
	}

	// Initial data load
	if err := updateData(); err != nil {
		log.Printf("Initial data load error: %v", err)
	}

	// Create keyboard event handler
	keyboardHandler := tui.CreateKeyboardHandler(cancel, updateData)

	// Run the dashboard
	currentRefresh := uiParams.Get()
	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(keyboardHandler), termdash.RedrawInterval(currentRefresh)); err != nil {
		log.Fatalf("termdash.Run => %v", err)
	}
}
