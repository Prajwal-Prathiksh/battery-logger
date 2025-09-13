// Package widgets provides custom chart widgets with enhanced functionality
package widgets

import (
	"fmt"
	"image"
	"math"
	"time"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/private/canvas"
	"github.com/mum4k/termdash/private/canvas/braille"
	"github.com/mum4k/termdash/private/draw"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
)

// TimePoint represents a single data point with timestamp
type TimePoint struct {
	Time  time.Time
	Value float64
	State bool // For battery: true=charging, false=discharging
}

// TimeSeries represents a series of time-based data points
type TimeSeries struct {
	Name   string
	Points []TimePoint
	Color  cell.Color
}

// TimeChart is a time-aware chart widget with day/night backgrounds
type TimeChart struct {
	series []TimeSeries
	window time.Duration
	yMin   float64
	yMax   float64
	yLabel string
	title  string

	// Day/night configuration
	dayColor   cell.Color
	nightColor cell.Color
	dayStart   int // hour 0-23
	dayEnd     int // hour 0-23

	// Date annotation settings
	showDates     bool
	dateThreshold time.Duration // minimum window size to show dates
}

// TimeChartOption is used to configure the TimeChart
type TimeChartOption interface {
	set(*TimeChart)
}

type timeChartOption func(*TimeChart)

func (o timeChartOption) set(tc *TimeChart) {
	o(tc)
}

// NewTimeChart creates a new time-aware chart widget
func NewTimeChart(opts ...TimeChartOption) *TimeChart {
	tc := &TimeChart{
		window: 24 * time.Hour,
		yMin:   0,
		yMax:   100,
		yLabel: "Battery %",
		title:  "Battery Over Time",

		// Sensible day/night color palette
		dayColor:   cell.ColorNumber(248), // Very light gray for day (subtle)
		nightColor: cell.ColorNumber(240), // Slightly darker gray for night
		dayStart:   6,                     // 6 AM
		dayEnd:     18,                    // 6 PM

		// Date annotation settings
		showDates:     true,
		dateThreshold: 0, // Always show dates regardless of window size
	}

	for _, opt := range opts {
		opt.set(tc)
	}

	return tc
}

// Option functions
func Window(d time.Duration) TimeChartOption {
	return timeChartOption(func(tc *TimeChart) {
		tc.window = d
	})
}

func YRange(min, max float64) TimeChartOption {
	return timeChartOption(func(tc *TimeChart) {
		tc.yMin = min
		tc.yMax = max
	})
}

func YLabel(label string) TimeChartOption {
	return timeChartOption(func(tc *TimeChart) {
		tc.yLabel = label
	})
}

func Title(title string) TimeChartOption {
	return timeChartOption(func(tc *TimeChart) {
		tc.title = title
	})
}

func DayNightColors(day, night cell.Color) TimeChartOption {
	return timeChartOption(func(tc *TimeChart) {
		tc.dayColor = day
		tc.nightColor = night
	})
}

func DayHours(start, end int) TimeChartOption {
	return timeChartOption(func(tc *TimeChart) {
		tc.dayStart = start
		tc.dayEnd = end
	})
}

// SetSeries sets the data series for the chart
func (tc *TimeChart) SetSeries(series []TimeSeries) {
	tc.series = series
}

// AddSeries adds a single series to the chart
func (tc *TimeChart) AddSeries(name string, points []TimePoint, color cell.Color) {
	tc.series = append(tc.series, TimeSeries{
		Name:   name,
		Points: points,
		Color:  color,
	})
}

// ClearSeries removes all series from the chart
func (tc *TimeChart) ClearSeries() {
	tc.series = tc.series[:0]
}

// SetWindow updates the time window for the chart
func (tc *TimeChart) SetWindow(window time.Duration) {
	tc.window = window
}

// Draw implements widgetapi.Widget.Draw
func (tc *TimeChart) Draw(cvs *canvas.Canvas, meta *widgetapi.Meta) error {
	if len(tc.series) == 0 {
		return draw.Text(cvs, "No data", image.Point{1, 1})
	}

	area := cvs.Area()
	if area.Dx() < 10 || area.Dy() < 5 {
		return draw.ResizeNeeded(cvs)
	}

	// Clear canvas
	cvs.Clear()

	// Calculate plot area (leaving space for axes and labels)
	plotArea := image.Rect(
		area.Min.X+5, // Y-axis labels
		area.Min.Y+1, // Title
		area.Max.X-1, // Right margin
		area.Max.Y-3, // X-axis labels
	)

	if plotArea.Dx() < 5 || plotArea.Dy() < 3 {
		return draw.ResizeNeeded(cvs)
	}

	// Calculate time range
	endTime := time.Now()
	startTime := endTime.Add(-tc.window)

	// Draw day/night background
	if err := tc.drawDayNightBackground(cvs, plotArea, startTime, endTime); err != nil {
		return err
	}

	// Draw axes
	if err := tc.drawAxes(cvs, area, plotArea); err != nil {
		return err
	}

	// Draw Y-axis labels
	if err := tc.drawYLabels(cvs, plotArea); err != nil {
		return err
	}

	// Draw X-axis labels (time)
	if err := tc.drawXLabels(cvs, plotArea, startTime, endTime); err != nil {
		return err
	}

	// Draw date annotations and midnight lines (always show)
	if tc.showDates {
		if err := tc.drawDateAnnotations(cvs, plotArea, startTime, endTime); err != nil {
			return err
		}
	}

	// Create braille canvas for high-resolution line drawing
	bc, err := braille.New(plotArea)
	if err != nil {
		return err
	}

	// Draw data series
	for _, series := range tc.series {
		if err := tc.drawSeries(bc, plotArea, series, startTime, endTime); err != nil {
			return err
		}
	}

	// Copy braille canvas to main canvas while preserving background colors
	return tc.copyBrailleWithBackground(bc, cvs, plotArea, startTime, endTime)
}

// drawDayNightBackground draws alternating day/night background colors
func (tc *TimeChart) drawDayNightBackground(cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	timeSpan := endTime.Sub(startTime)
	if timeSpan <= 0 {
		return nil
	}

	// Calculate time per pixel
	pixelWidth := plotArea.Dx()
	timePerPixel := timeSpan / time.Duration(pixelWidth)

	for x := plotArea.Min.X; x < plotArea.Max.X; x++ {
		// Calculate the time for this x position
		pixelTime := startTime.Add(time.Duration(x-plotArea.Min.X) * timePerPixel)
		hour := pixelTime.Hour()

		// Determine if it's day or night
		var bgColor cell.Color
		if hour >= tc.dayStart && hour < tc.dayEnd {
			bgColor = tc.dayColor
		} else {
			bgColor = tc.nightColor
		}

		// Fill the column
		for y := plotArea.Min.Y; y < plotArea.Max.Y; y++ {
			cvs.SetCellOpts(image.Point{x, y}, cell.BgColor(bgColor))
		}
	}

	return nil
}

// drawAxes draws the X and Y axes
func (tc *TimeChart) drawAxes(cvs *canvas.Canvas, area, plotArea image.Rectangle) error {
	// Y-axis (left side)
	yAxisLine := []draw.HVLine{{
		Start: image.Point{plotArea.Min.X - 1, plotArea.Min.Y},
		End:   image.Point{plotArea.Min.X - 1, plotArea.Max.Y - 1},
	}}

	// X-axis (bottom)
	xAxisLine := []draw.HVLine{{
		Start: image.Point{plotArea.Min.X - 1, plotArea.Max.Y},
		End:   image.Point{plotArea.Max.X - 1, plotArea.Max.Y},
	}}

	if err := draw.HVLines(cvs, yAxisLine); err != nil {
		return err
	}

	return draw.HVLines(cvs, xAxisLine)
}

// drawYLabels draws Y-axis value labels
func (tc *TimeChart) drawYLabels(cvs *canvas.Canvas, plotArea image.Rectangle) error {
	height := plotArea.Dy()
	if height < 3 {
		return nil
	}

	// Draw 3-5 Y labels
	numLabels := 4
	for i := 0; i < numLabels; i++ {
		y := plotArea.Max.Y - 1 - (i * height / (numLabels - 1))
		if y < plotArea.Min.Y {
			continue
		}

		value := tc.yMin + (tc.yMax-tc.yMin)*float64(i)/float64(numLabels-1)
		label := fmt.Sprintf("%.0f", value)

		// Position label to the left of the Y-axis
		labelPos := image.Point{plotArea.Min.X - len(label) - 1, y}
		if labelPos.X >= 0 {
			draw.Text(cvs, label, labelPos, draw.TextCellOpts(cell.FgColor(cell.ColorCyan)))
		}
	}

	return nil
}

// drawXLabels draws X-axis time labels
func (tc *TimeChart) drawXLabels(cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	width := plotArea.Dx()
	if width < 10 {
		return nil
	}

	// Determine label frequency based on window size
	var labelInterval time.Duration
	if tc.window <= 2*time.Hour {
		labelInterval = 30 * time.Minute
	} else if tc.window <= 12*time.Hour {
		labelInterval = 2 * time.Hour
	} else if tc.window <= 48*time.Hour {
		labelInterval = 6 * time.Hour
	} else {
		labelInterval = 12 * time.Hour
	}

	// Draw time labels
	timeSpan := endTime.Sub(startTime)
	for t := startTime.Truncate(labelInterval).Add(labelInterval); t.Before(endTime); t = t.Add(labelInterval) {
		if t.Before(startTime) {
			continue
		}

		x := plotArea.Min.X + int(float64(width)*t.Sub(startTime).Seconds()/timeSpan.Seconds())
		if x >= plotArea.Max.X {
			break
		}

		var label string
		if tc.window <= 12*time.Hour {
			label = t.Format("15:04")
		} else {
			label = t.Format("15:04")
		}

		labelPos := image.Point{x - len(label)/2, plotArea.Max.Y + 1}
		draw.Text(cvs, label, labelPos, draw.TextCellOpts(cell.FgColor(cell.ColorCyan)))
	}

	return nil
}

// drawDateAnnotations draws date stamps and midnight lines for any time window
func (tc *TimeChart) drawDateAnnotations(cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	timeSpan := endTime.Sub(startTime)
	width := plotArea.Dx()

	// Find midnight boundaries within the time range
	current := startTime.Truncate(24 * time.Hour)
	if current.Before(startTime) {
		current = current.Add(24 * time.Hour)
	}

	for current.Before(endTime) {
		x := plotArea.Min.X + int(float64(width)*current.Sub(startTime).Seconds()/timeSpan.Seconds())
		if x >= plotArea.Min.X && x < plotArea.Max.X {
			// Draw a thin vertical line at midnight (day break)
			for y := plotArea.Min.Y; y < plotArea.Max.Y; y++ {
				cvs.SetCellOpts(image.Point{x, y}, cell.FgColor(cell.ColorNumber(244)))
			}

			// Draw date label above the chart
			dateLabel := current.Format("Jan 2")
			labelPos := image.Point{x - len(dateLabel)/2, plotArea.Min.Y - 1}
			if labelPos.Y >= 0 {
				draw.Text(cvs, dateLabel, labelPos,
					draw.TextCellOpts(cell.FgColor(cell.ColorWhite), cell.Bold()))
			}
		}
		current = current.Add(24 * time.Hour)
	}

	return nil
}

// drawSeries draws a single data series using braille canvas with proper gap handling
func (tc *TimeChart) drawSeries(bc *braille.Canvas, plotArea image.Rectangle, series TimeSeries, startTime, endTime time.Time) error {
	if len(series.Points) == 0 {
		return nil
	}

	timeSpan := endTime.Sub(startTime)
	if timeSpan <= 0 {
		return nil
	}

	// Convert braille canvas coordinates (higher resolution)
	brailleArea := bc.Area()
	brailleWidth := brailleArea.Dx()
	brailleHeight := brailleArea.Dy()

	var prevPoint *image.Point
	var prevTime time.Time

	for _, point := range series.Points {
		if point.Time.Before(startTime) || point.Time.After(endTime) {
			continue
		}

		// Skip NaN values
		if math.IsNaN(point.Value) {
			prevPoint = nil
			continue
		}

		// Calculate pixel coordinates in braille space
		x := int(float64(brailleWidth) * point.Time.Sub(startTime).Seconds() / timeSpan.Seconds())
		y := brailleHeight - 1 - int(float64(brailleHeight)*(point.Value-tc.yMin)/(tc.yMax-tc.yMin))

		// Clamp to bounds
		if x < 0 || x >= brailleWidth || y < 0 || y >= brailleHeight {
			prevPoint = nil
			continue
		}

		currentPoint := image.Point{x, y}

		// Draw line from previous point if it exists AND the time gap is reasonable
		if prevPoint != nil {
			// Check if there's a significant time gap (more than 5 minutes)
			timeGap := point.Time.Sub(prevTime)
			maxGap := 5 * time.Minute

			// Only draw line if the time gap is reasonable (continuous data)
			if timeGap <= maxGap {
				draw.BrailleLine(bc, *prevPoint, currentPoint,
					draw.BrailleLineCellOpts(cell.FgColor(series.Color)))
			} else {
				// Just draw the point (creating a gap in the line)
				bc.SetPixel(currentPoint, cell.FgColor(series.Color))
			}
		} else {
			// Just draw the point
			bc.SetPixel(currentPoint, cell.FgColor(series.Color))
		}

		prevPoint = &currentPoint
		prevTime = point.Time
	}

	return nil
}

// copyBrailleWithBackground copies braille canvas while preserving day/night background colors
func (tc *TimeChart) copyBrailleWithBackground(bc *braille.Canvas, cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	timeSpan := endTime.Sub(startTime)
	if timeSpan <= 0 {
		return bc.CopyTo(cvs)
	}

	// First copy the braille content normally
	if err := bc.CopyTo(cvs); err != nil {
		return err
	}

	// Then apply background colors to the entire plot area
	// This approach overlays background colors while preserving foreground content
	for y := plotArea.Min.Y; y < plotArea.Max.Y; y++ {
		for x := plotArea.Min.X; x < plotArea.Max.X; x++ {
			// Calculate the time for this x position to determine background color
			pixelTime := startTime.Add(time.Duration(x-plotArea.Min.X) * timeSpan / time.Duration(plotArea.Dx()))
			hour := pixelTime.Hour()

			// Determine background color
			var bgColor cell.Color
			if hour >= tc.dayStart && hour < tc.dayEnd {
				bgColor = tc.dayColor
			} else {
				bgColor = tc.nightColor
			}

			// Apply background color to this cell
			// This preserves any existing foreground content from the braille canvas
			cvs.SetCellOpts(image.Point{x, y}, cell.BgColor(bgColor))
		}
	}

	return nil
}

// Keyboard implements widgetapi.Widget.Keyboard
func (tc *TimeChart) Keyboard(k *terminalapi.Keyboard, meta *widgetapi.EventMeta) error {
	return nil
}

// Mouse implements widgetapi.Widget.Mouse
func (tc *TimeChart) Mouse(m *terminalapi.Mouse, meta *widgetapi.EventMeta) error {
	return nil
}

// Options implements widgetapi.Widget.Options
func (tc *TimeChart) Options() widgetapi.Options {
	return widgetapi.Options{
		WantKeyboard: widgetapi.KeyScopeNone,
		WantMouse:    widgetapi.MouseScopeNone,
	}
}
