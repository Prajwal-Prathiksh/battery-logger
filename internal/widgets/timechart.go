// Package widgets provides custom chart widgets with enhanced functionality
package widgets

import (
	"fmt"
	"image"
	"math"
	"time"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/mouse"
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

// BatteryChart is a time-aware chart widget with day/night backgrounds and zoom functionality
type BatteryChart struct {
	series []TimeSeries
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

	// Zoom and navigation state
	baseWindow    time.Duration // base window size (10h default)
	currentWindow time.Duration // current zoomed window
	windowStart   time.Time     // start time of current view
	windowEnd     time.Time     // end time of current view

	// Mouse state for drag selection
	isDragging bool
	dragStart  image.Point
	dragEnd    image.Point

	// Zoom parameters
	zoomStep  float64       // zoom step percentage (0.1 = 10%)
	minWindow time.Duration // minimum zoom window (5m)
	maxWindow time.Duration // maximum zoom window (7d)

	// Data bounds for limiting pan operations
	dataStart time.Time // earliest data point
	dataEnd   time.Time // latest data point

	// Callback for when zoom/pan changes
	onZoomChange func(startTime time.Time, endTime time.Time, duration time.Duration)
}

// BatteryChartOption is used to configure the BatteryChart
type BatteryChartOption interface {
	set(*BatteryChart)
}

type batteryChartOption func(*BatteryChart)

func (o batteryChartOption) set(tc *BatteryChart) {
	o(tc)
}

// CreateBatteryChart creates a time-aware chart widget with zoom functionality
func CreateBatteryChart(opts ...BatteryChartOption) *BatteryChart {
	tc := &BatteryChart{
		baseWindow:    24 * time.Hour,
		currentWindow: 24 * time.Hour,
		yMin:          0,
		yMax:          100,
		yLabel:        "Battery %",
		title:         "Battery Over Time",

		// High contrast day/night color palette
		dayColor:   cell.ColorNumber(237), // Dark gray for day (darker but still distinguishable)
		nightColor: cell.ColorNumber(0),   // True black for night (pitch black)
		dayStart:   7,                     // 7 AM
		dayEnd:     19,                    // 7 PM

		// Date annotation settings
		showDates:     true,
		dateThreshold: 0, // Always show dates regardless of window size

		// Zoom parameters
		zoomStep:  0.1,                // 10% zoom steps
		minWindow: 5 * time.Minute,    // minimum 5 minutes
		maxWindow: 7 * 24 * time.Hour, // maximum 7 days
	}

	// Initialize current view to the base window
	now := time.Now()
	tc.windowEnd = now
	tc.windowStart = now.Add(-tc.baseWindow)

	for _, opt := range opts {
		opt.set(tc)
	}

	return tc
}

// Option functions
func Window(d time.Duration) BatteryChartOption {
	return batteryChartOption(func(tc *BatteryChart) {
		tc.baseWindow = d
		tc.currentWindow = d
	})
}

func YRange(min, max float64) BatteryChartOption {
	return batteryChartOption(func(tc *BatteryChart) {
		tc.yMin = min
		tc.yMax = max
	})
}

func YLabel(label string) BatteryChartOption {
	return batteryChartOption(func(tc *BatteryChart) {
		tc.yLabel = label
	})
}

func Title(title string) BatteryChartOption {
	return batteryChartOption(func(tc *BatteryChart) {
		tc.title = title
	})
}

func DayNightColors(day, night cell.Color) BatteryChartOption {
	return batteryChartOption(func(tc *BatteryChart) {
		tc.dayColor = day
		tc.nightColor = night
	})
}

func DayHours(start, end int) BatteryChartOption {
	return batteryChartOption(func(tc *BatteryChart) {
		tc.dayStart = start
		tc.dayEnd = end
	})
}

// SetSeries sets the data series for the chart
func (tc *BatteryChart) SetSeries(series []TimeSeries) {
	tc.series = series
	tc.updateDataBounds()
}

// updateDataBounds calculates and stores the earliest and latest data points
func (tc *BatteryChart) updateDataBounds() {
	if len(tc.series) == 0 {
		return
	}

	// Find the earliest and latest data points across all series
	var earliest, latest time.Time
	first := true

	for _, s := range tc.series {
		for _, p := range s.Points {
			if first {
				earliest = p.Time
				latest = p.Time
				first = false
			} else {
				if p.Time.Before(earliest) {
					earliest = p.Time
				}
				if p.Time.After(latest) {
					latest = p.Time
				}
			}
		}
	}

	tc.dataStart = earliest
	tc.dataEnd = latest
}

// AddSeries adds a single series to the chart
func (tc *BatteryChart) AddSeries(name string, points []TimePoint, color cell.Color) {
	tc.series = append(tc.series, TimeSeries{
		Name:   name,
		Points: points,
		Color:  color,
	})
	tc.updateDataBounds()
}

// ClearSeries removes all series from the chart
func (tc *BatteryChart) ClearSeries() {
	tc.series = tc.series[:0]
	// Reset data bounds when clearing series
	tc.dataStart = time.Time{}
	tc.dataEnd = time.Time{}
}

// SetWindow updates the base time window for the chart
func (tc *BatteryChart) SetWindow(window time.Duration) {
	tc.baseWindow = window
	tc.currentWindow = window
	// Update window times
	now := time.Now()
	tc.windowEnd = now
	tc.windowStart = now.Add(-window)
}

// SetOnZoomChange sets a callback that is called whenever the zoom or pan changes
func (tc *BatteryChart) SetOnZoomChange(callback func(startTime time.Time, endTime time.Time, duration time.Duration)) {
	tc.onZoomChange = callback
}

// triggerZoomChange calls the zoom change callback if it's set
func (tc *BatteryChart) triggerZoomChange() {
	if tc.onZoomChange != nil {
		tc.onZoomChange(tc.windowStart, tc.windowEnd, tc.currentWindow)
	}
}

// GetCurrentWindow returns the current zoom window information
func (tc *BatteryChart) GetCurrentWindow() (startTime time.Time, endTime time.Time, duration time.Duration) {
	return tc.windowStart, tc.windowEnd, tc.currentWindow
}

// Draw implements widgetapi.Widget.Draw
func (tc *BatteryChart) Draw(cvs *canvas.Canvas, meta *widgetapi.Meta) error {
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

	// Calculate time range - use current zoom window
	endTime := tc.windowEnd
	startTime := tc.windowStart

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

	// Draw date annotations (labels only, before braille)
	if tc.showDates {
		if err := tc.drawDateLabels(cvs, plotArea, startTime, endTime); err != nil {
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
	if err := tc.copyBrailleWithBackground(bc, cvs, plotArea, startTime, endTime); err != nil {
		return err
	}

	// Draw day-break lines AFTER braille copy so they appear on top
	if tc.showDates {
		return tc.drawDayBreakLines(cvs, plotArea, startTime, endTime)
	}

	return nil
}

// drawDayNightBackground draws alternating day/night background colors
func (tc *BatteryChart) drawDayNightBackground(cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
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
		if hour >= tc.dayStart && hour < tc.dayEnd {
			// Only fill day areas with light gray background
			// Leave night areas untouched (transparent) for natural black appearance
			for y := plotArea.Min.Y; y < plotArea.Max.Y; y++ {
				cvs.SetCellOpts(image.Point{x, y}, cell.BgColor(tc.dayColor))
			}
		}
		// Night areas are left unfilled (transparent) for natural terminal black
	}

	return nil
}

// drawAxes draws the X and Y axes
func (tc *BatteryChart) drawAxes(cvs *canvas.Canvas, area, plotArea image.Rectangle) error {
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
func (tc *BatteryChart) drawYLabels(cvs *canvas.Canvas, plotArea image.Rectangle) error {
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
		label := fmt.Sprintf("%.0f%%", value)

		// Position label to the left of the Y-axis
		labelPos := image.Point{plotArea.Min.X - len(label) - 1, y}
		if labelPos.X >= 0 {
			draw.Text(cvs, label, labelPos, draw.TextCellOpts(cell.FgColor(cell.ColorCyan)))
		}
	}

	return nil
}

// drawXLabels draws X-axis time labels
func (tc *BatteryChart) drawXLabels(cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	width := plotArea.Dx()
	if width < 10 {
		return nil
	}

	// Determine label frequency based on window size with more granular intervals
	var labelInterval time.Duration
	if tc.currentWindow <= 30*time.Minute {
		labelInterval = 5 * time.Minute
	} else if tc.currentWindow <= 2*time.Hour {
		labelInterval = 15 * time.Minute
	} else if tc.currentWindow <= 4*time.Hour {
		labelInterval = 30 * time.Minute
	} else if tc.currentWindow <= 8*time.Hour {
		labelInterval = 1 * time.Hour
	} else if tc.currentWindow <= 24*time.Hour {
		labelInterval = 2 * time.Hour
	} else if tc.currentWindow <= 48*time.Hour {
		labelInterval = 4 * time.Hour
	} else if tc.currentWindow <= 7*24*time.Hour { // 1 week
		labelInterval = 12 * time.Hour
	} else {
		labelInterval = 24 * time.Hour
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
		if tc.currentWindow <= 4*time.Hour {
			// For very short windows, show minutes too
			label = t.Format("15:04")
		} else {
			// For longer windows, just show time
			label = t.Format("15:04")
		}

		labelPos := image.Point{x - len(label)/2, plotArea.Max.Y + 1}
		draw.Text(cvs, label, labelPos, draw.TextCellOpts(cell.FgColor(cell.ColorCyan)))
	}

	return nil
}

// drawDateLabels draws date stamps above the chart (no vertical lines)
func (tc *BatteryChart) drawDateLabels(cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	timeSpan := endTime.Sub(startTime)
	width := plotArea.Dx()

	// Find midnight boundaries within the time range
	// Start from the beginning of the day containing startTime
	year, month, day := startTime.Date()
	current := time.Date(year, month, day, 0, 0, 0, 0, startTime.Location())

	// If this midnight is before our start time, move to next midnight
	if current.Before(startTime) || current.Equal(startTime) {
		current = current.Add(24 * time.Hour)
	}

	for current.Before(endTime) {
		x := plotArea.Min.X + int(float64(width)*current.Sub(startTime).Seconds()/timeSpan.Seconds())
		if x >= plotArea.Min.X && x < plotArea.Max.X {
			// Draw date label above the chart
			dateLabel := current.Format("Jan 2")
			labelPos := image.Point{x - len(dateLabel)/2, plotArea.Min.Y - 1}
			if labelPos.Y >= 0 {
				draw.Text(cvs, dateLabel, labelPos,
					draw.TextCellOpts(cell.FgColor(cell.ColorCyan), cell.Bold()))
			}
		}
		current = current.Add(24 * time.Hour)
	}

	return nil
}

// drawDayBreakLines draws bright dashed vertical lines at midnight (drawn on top)
func (tc *BatteryChart) drawDayBreakLines(cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	timeSpan := endTime.Sub(startTime)
	width := plotArea.Dx()

	// Find midnight boundaries within the time range
	// Start from the beginning of the day containing startTime
	year, month, day := startTime.Date()
	current := time.Date(year, month, day, 0, 0, 0, 0, startTime.Location())

	// If this midnight is before our start time, move to next midnight
	if current.Before(startTime) || current.Equal(startTime) {
		current = current.Add(24 * time.Hour)
	}

	for current.Before(endTime) {
		x := plotArea.Min.X + int(float64(width)*current.Sub(startTime).Seconds()/timeSpan.Seconds())
		if x >= plotArea.Min.X && x < plotArea.Max.X {
			// Draw a bright dashed vertical line at midnight (day break)
			for y := plotArea.Min.Y; y < plotArea.Max.Y; y++ {
				// Create dashed effect by alternating characters every 2 rows
				if y%2 == 0 {
					cvs.SetCell(image.Point{x, y}, '┊', cell.FgColor(cell.ColorCyan))
				} else {
					cvs.SetCell(image.Point{x, y}, ' ', cell.FgColor(cell.ColorCyan))
				}
			}
		}
		current = current.Add(24 * time.Hour)
	}

	return nil
}

// drawSeries draws a single data series using braille canvas with proper gap handling
func (tc *BatteryChart) drawSeries(bc *braille.Canvas, plotArea image.Rectangle, series TimeSeries, startTime, endTime time.Time) error {
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
func (tc *BatteryChart) copyBrailleWithBackground(bc *braille.Canvas, cvs *canvas.Canvas, plotArea image.Rectangle, startTime, endTime time.Time) error {
	timeSpan := endTime.Sub(startTime)
	if timeSpan <= 0 {
		return bc.CopyTo(cvs)
	}

	// First copy the braille content normally
	if err := bc.CopyTo(cvs); err != nil {
		return err
	}

	// Then apply background colors only to day areas
	// Leave night areas untouched for natural terminal black
	for y := plotArea.Min.Y; y < plotArea.Max.Y; y++ {
		for x := plotArea.Min.X; x < plotArea.Max.X; x++ {
			// Calculate the time for this x position to determine if it's day
			pixelTime := startTime.Add(time.Duration(x-plotArea.Min.X) * timeSpan / time.Duration(plotArea.Dx()))
			hour := pixelTime.Hour()

			// Only apply background color during day hours
			// Night areas remain untouched (transparent) for natural black
			if hour >= tc.dayStart && hour < tc.dayEnd {
				cvs.SetCellOpts(image.Point{x, y}, cell.BgColor(tc.dayColor))
			}
		}
	}

	return nil
}

// Keyboard implements widgetapi.Widget.Keyboard
func (tc *BatteryChart) Keyboard(k *terminalapi.Keyboard, meta *widgetapi.EventMeta) error {
	switch k.Key {
	case keyboard.KeyArrowLeft:
		// Pan left (backward in time)
		return tc.pan(false)
	case keyboard.KeyArrowRight:
		// Pan right (forward in time)
		return tc.pan(true)
	case 'i', 'I':
		// Zoom in (reduce window size)
		return tc.zoom(true, image.Point{})
	case 'o', 'O':
		// Zoom out (increase window size)
		return tc.zoom(false, image.Point{})
	case keyboard.KeyEsc:
		// Reset zoom to base window
		tc.currentWindow = tc.baseWindow
		now := time.Now()
		tc.windowEnd = now
		tc.windowStart = now.Add(-tc.baseWindow)
		tc.triggerZoomChange()
		return nil
	}
	return nil
}

// Mouse implements widgetapi.Widget.Mouse
func (tc *BatteryChart) Mouse(m *terminalapi.Mouse, meta *widgetapi.EventMeta) error {
	switch m.Button {
	case mouse.ButtonWheelUp:
		// Zoom in (reduce window size)
		return tc.zoom(true, m.Position)
	case mouse.ButtonWheelDown:
		// Zoom out (increase window size)
		return tc.zoom(false, m.Position)
	case mouse.ButtonLeft:
		// Start drag selection
		tc.isDragging = true
		tc.dragStart = m.Position
		tc.dragEnd = m.Position
	case mouse.ButtonRelease:
		// End drag selection and zoom to selected area
		if tc.isDragging {
			tc.isDragging = false
			return tc.zoomToSelection()
		}
	}

	// Update drag end position while dragging
	if tc.isDragging && m.Button == mouse.ButtonLeft {
		tc.dragEnd = m.Position
	}

	return nil
}

// Options implements widgetapi.Widget.Options
func (tc *BatteryChart) Options() widgetapi.Options {
	return widgetapi.Options{
		WantKeyboard: widgetapi.KeyScopeGlobal,
		WantMouse:    widgetapi.MouseScopeGlobal,
	}
}

// zoom handles mouse wheel zoom in/out
func (tc *BatteryChart) zoom(zoomIn bool, position image.Point) error {
	if zoomIn {
		// Zoom in: reduce window size
		newWindow := time.Duration(float64(tc.currentWindow) * (1.0 - tc.zoomStep))
		if newWindow < tc.minWindow {
			newWindow = tc.minWindow
		}
		tc.currentWindow = newWindow
	} else {
		// Zoom out: increase window size
		newWindow := time.Duration(float64(tc.currentWindow) * (1.0 + tc.zoomStep))
		if newWindow > tc.maxWindow {
			newWindow = tc.maxWindow
		}
		tc.currentWindow = newWindow
	}

	// Update window times (keep end time, adjust start time)
	tc.windowStart = tc.windowEnd.Add(-tc.currentWindow)

	// Trigger callback to update title
	tc.triggerZoomChange()
	return nil
}

// zoomToSelection zooms to the time range selected by mouse drag
func (tc *BatteryChart) zoomToSelection() error {
	if tc.dragStart.X == tc.dragEnd.X {
		// No selection made, ignore
		return nil
	}

	// TODO: Convert pixel coordinates to time range and update windowStart/windowEnd
	// For now, just clear the drag state
	tc.isDragging = false
	return nil
}

// pan moves the view left/right while maintaining zoom level
func (tc *BatteryChart) pan(right bool) error {
	// Don't pan if no data bounds are set
	if tc.dataStart.IsZero() || tc.dataEnd.IsZero() {
		return nil
	}

	// Pan distance is 10% of current window
	panDistance := time.Duration(float64(tc.currentWindow) * 0.1)

	var newStart, newEnd time.Time
	if right {
		newStart = tc.windowStart.Add(panDistance)
		newEnd = tc.windowEnd.Add(panDistance)
	} else {
		newStart = tc.windowStart.Add(-panDistance)
		newEnd = tc.windowEnd.Add(-panDistance)
	}

	// Check bounds and limit panning
	// Don't allow the view to go beyond the data range
	if newStart.Before(tc.dataStart) {
		// Adjust to keep the start at data start
		newStart = tc.dataStart
		newEnd = newStart.Add(tc.currentWindow)
	}
	if newEnd.After(tc.dataEnd) {
		// Adjust to keep the end at data end
		newEnd = tc.dataEnd
		newStart = newEnd.Add(-tc.currentWindow)
		// If the window is larger than the data range, center it
		if newStart.Before(tc.dataStart) {
			newStart = tc.dataStart
		}
	}

	tc.windowStart = newStart
	tc.windowEnd = newEnd

	// Trigger callback to update title
	tc.triggerZoomChange()
	return nil
}
