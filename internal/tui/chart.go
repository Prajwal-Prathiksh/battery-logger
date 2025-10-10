package tui

import (
	"fmt"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/widgets"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
)

// ProcessChartData converts battery data to BatteryChart format
func ProcessChartData(rows []analytics.Row) ([]widgets.TimeSeries, error) {
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

// UpdateChartWidget updates the chart widget with new data
func UpdateChartWidget(chartWidget *widgets.BatteryChart, series []widgets.TimeSeries) error {
	chartWidget.ClearSeries()
	chartWidget.SetSeries(series)
	return nil
}

// UpdateChartTitleFromZoom updates the chart title with the current zoom duration
func UpdateChartTitleFromZoom(c *container.Container, startTime, endTime time.Time) {
	timeDiff := endTime.Sub(startTime)
	span := FormatDurationAuto(timeDiff.Round(time.Minute))
	title := fmt.Sprintf("Battery %% Over Time [%s] - i/o/mouse wheel: zoom, ←→: pan, esc: reset", span)
	c.Update("chart-container", container.BorderTitle(title))
}
