package tui

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/config"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/widgets"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/text"
)

// SetupDataRefresh sets up periodic data refresh and returns the update function
func SetupDataRefresh(ctx context.Context, logPath string, uiParams *UIParams, chartWidget *widgets.BatteryChart, textWidget *text.Text, sotBarChart *widgets.SOTBarChart, cfg config.Config, c *container.Container, alpha float64, readCSVFunc func(string) ([]analytics.Row, error)) (func() error, error) {
	updateData := func() error {
		rows, err := readCSVFunc(logPath)
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
		series, err := ProcessChartData(rows)
		if err != nil {
			return fmt.Errorf("processing chart data: %v", err)
		}

		// Update chart
		if err := UpdateChartWidget(chartWidget, series); err != nil {
			return fmt.Errorf("updating chart: %v", err)
		}

		// Update chart title with current zoom window (not full data range)
		startTime, endTime, _ := chartWidget.GetCurrentWindow()
		UpdateChartTitleFromZoom(c, startTime, endTime)

		// Generate and update status text
		statusInfo := GenerateStatusInfo(rows, alpha, uiParams, logPath, cfg)
		UpdateStatusText(textWidget, statusInfo)

		// Update SOT bar chart
		if err := UpdateSOTBarChart(sotBarChart, rows, cfg.SuspendGapMinutes); err != nil {
			return fmt.Errorf("updating SOT bar chart: %v", err)
		}

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

// CreateKeyboardHandler creates the keyboard event handler for the TUI
func CreateKeyboardHandler(cancel context.CancelFunc, updateData func() error) func(*terminalapi.Keyboard) {
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
