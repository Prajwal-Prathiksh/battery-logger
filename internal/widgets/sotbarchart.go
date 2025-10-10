// Package widgets provides custom chart widgets with enhanced functionality
package widgets

import (
	"fmt"
	"image"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/private/canvas"
	"github.com/mum4k/termdash/private/draw"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
)

// SOTBarData represents daily screen-on time data for a single day
type SOTBarData struct {
	Date        time.Time
	SOTDuration time.Duration
	IsToday     bool
	HasData     bool
}

// SOTBarChart displays daily screen-on time as bars with HH:MM annotations
type SOTBarChart struct {
	data  []SOTBarData
	title string

	// Colors
	barColor      cell.Color
	todayBarColor cell.Color
	textColor     cell.Color
	titleColor    cell.Color
}

// SOTBarChartOption is used to configure the SOTBarChart
type SOTBarChartOption interface {
	setSOTBar(*SOTBarChart)
}

type sotBarChartOption func(*SOTBarChart)

func (o sotBarChartOption) setSOTBar(bc *SOTBarChart) {
	o(bc)
}

// CreateSOTBarChart creates a new screen-on time bar chart widget
func CreateSOTBarChart(opts ...SOTBarChartOption) *SOTBarChart {
	bc := &SOTBarChart{
		title:         "Daily Screen-On Time (7 days)",
		barColor:      cell.ColorCyan,
		todayBarColor: cell.ColorYellow,
		textColor:     cell.ColorWhite,
		titleColor:    cell.ColorCyan,
	}

	for _, opt := range opts {
		opt.setSOTBar(bc)
	}

	return bc
}

// SOTBarTitle sets the title of the bar chart
func SOTBarTitle(title string) SOTBarChartOption {
	return sotBarChartOption(func(bc *SOTBarChart) {
		bc.title = title
	})
}

// SOTBarColors sets the colors for the bar chart
func SOTBarColors(barColor, todayBarColor, textColor cell.Color) SOTBarChartOption {
	return sotBarChartOption(func(bc *SOTBarChart) {
		bc.barColor = barColor
		bc.todayBarColor = todayBarColor
		bc.textColor = textColor
	})
}

// UpdateData updates the SOT data for the past 7 days
func (bc *SOTBarChart) UpdateData(rows []analytics.Row, gapThresholdMinutes int) {
	now := time.Now()
	var weekData []SOTBarData

	// Calculate for the past 7 days (including today)
	for i := 6; i >= 0; i-- {
		date := now.AddDate(0, 0, -i)
		sotResult := analytics.CalculateDailyScreenOnTime(rows, date, gapThresholdMinutes)

		weekData = append(weekData, SOTBarData{
			Date:        date,
			SOTDuration: sotResult.TotalActiveTime,
			IsToday:     i == 0,
			HasData:     sotResult.TotalActiveTime > 0,
		})
	}

	bc.data = weekData
}

// formatDuration formats a duration to HH:MM format
func (bc *SOTBarChart) formatDuration(d time.Duration) string {
	if d <= 0 {
		return "00:00"
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%02d:%02d", hours, minutes)
}

// Draw implements widgetapi.Widget.Draw
func (bc *SOTBarChart) Draw(cvs *canvas.Canvas, meta *widgetapi.Meta) error {
	area := cvs.Area()
	if area.Dx() < 10 || area.Dy() < 5 {
		return draw.ResizeNeeded(cvs)
	}

	// Clear canvas
	cvs.Clear()

	if len(bc.data) == 0 {
		return draw.Text(cvs, "No SOT data", image.Point{1, 1})
	}

	// Calculate drawing areas within the provided canvas area
	// Leave space for time labels above bars and day labels below
	topLabelHeight := 1
	bottomLabelHeight := 1

	// Bar drawing area
	barArea := image.Rect(
		area.Min.X,
		area.Min.Y+topLabelHeight, // Space for time labels above
		area.Max.X,
		area.Max.Y-bottomLabelHeight, // Space for day labels below
	)

	if barArea.Dy() < 1 {
		return draw.ResizeNeeded(cvs)
	}

	// Calculate bar dimensions with proper spacing
	numBars := len(bc.data)
	totalWidth := barArea.Dx()

	// Reserve space for gaps between bars
	barSpacing := 1
	totalSpacing := barSpacing * (numBars - 1) // gaps between bars
	availableWidth := totalWidth - totalSpacing
	barWidth := availableWidth / numBars

	if barWidth < 1 {
		barWidth = 1
		barSpacing = 0 // No spacing if bars are too narrow
	}

	// Find max duration for scaling
	var maxDuration time.Duration
	for _, data := range bc.data {
		if data.SOTDuration > maxDuration {
			maxDuration = data.SOTDuration
		}
	}

	// Set minimum scale (1 hour)
	if maxDuration < time.Hour {
		maxDuration = time.Hour
	}

	// Draw each bar and its labels
	for i, data := range bc.data {
		// Calculate bar position with spacing
		barX := barArea.Min.X + i*(barWidth+barSpacing)
		barEndX := barX + barWidth

		// Calculate bar height based on SOT duration
		barHeight := 0
		if maxDuration > 0 {
			barHeight = int(float64(barArea.Dy()) * data.SOTDuration.Seconds() / maxDuration.Seconds())
		}

		// Choose bar color
		barColor := bc.barColor
		if data.IsToday {
			barColor = bc.todayBarColor
		}

		// Draw the bar
		barTop := barArea.Max.Y - barHeight
		for y := barTop; y < barArea.Max.Y; y++ {
			for x := barX; x < barEndX; x++ {
				if x >= barArea.Min.X && x < barArea.Max.X {
					cvs.SetCell(image.Point{x, y}, '█', cell.FgColor(barColor))
				}
			}
		}

		// Calculate center of bar for label alignment
		barCenter := barX + barWidth/2

		// Draw time label ABOVE the bar (positioned at the top of the bar)
		timeLabel := bc.formatDuration(data.SOTDuration)
		timeLabelX := barCenter - len(timeLabel)/2
		if timeLabelX >= area.Min.X && timeLabelX+len(timeLabel) <= area.Max.X {
			// Position above the top of the bar
			timeLabelY := barTop - 1
			if timeLabelY < area.Min.Y {
				timeLabelY = area.Min.Y
			}
			timeLabelPos := image.Point{timeLabelX, timeLabelY}
			draw.Text(cvs, timeLabel, timeLabelPos, draw.TextCellOpts(cell.FgColor(bc.textColor)))
		}

		// Draw day label below the bar
		var dayLabel string
		if data.IsToday {
			dayLabel = "Today"
		} else {
			dayLabel = data.Date.Format("Mon")
		}

		dayLabelX := barCenter - len(dayLabel)/2
		if dayLabelX >= area.Min.X && dayLabelX+len(dayLabel) <= area.Max.X {
			dayLabelPos := image.Point{dayLabelX, area.Max.Y - 1}
			draw.Text(cvs, dayLabel, dayLabelPos, draw.TextCellOpts(cell.FgColor(bc.textColor)))
		}
	}

	return nil
}

// Keyboard implements widgetapi.Widget.Keyboard (no keyboard interaction needed)
func (bc *SOTBarChart) Keyboard(k *terminalapi.Keyboard, meta *widgetapi.EventMeta) error {
	return nil
}

// Mouse implements widgetapi.Widget.Mouse (no mouse interaction needed)
func (bc *SOTBarChart) Mouse(m *terminalapi.Mouse, meta *widgetapi.EventMeta) error {
	return nil
}

// Options implements widgetapi.Widget.Options
func (bc *SOTBarChart) Options() widgetapi.Options {
	return widgetapi.Options{
		// No keyboard or mouse input needed
		WantKeyboard: widgetapi.KeyScopeNone,
		WantMouse:    widgetapi.MouseScopeNone,
		// Minimum size for reasonable display
		MinimumSize: image.Point{20, 8},
	}
}
