package tui

import (
	"github.com/Prajwal-Prathiksh/battery-logger/internal/config"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/widgets"

	"time"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/keyboard"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/text"
)

// CreateChartWidget creates and configures the time chart widget
func CreateChartWidget(cfg config.Config) *widgets.BatteryChart {
	return widgets.CreateBatteryChart(
		widgets.YRange(0, 100),
		widgets.YLabel("%"),
		widgets.Title("Battery % Over Time"),
		widgets.DayHours(cfg.DayStartHour, cfg.DayEndHour),
		widgets.DayNightColors(
			cell.ColorNumber(cfg.DayColorNumber),   // Day color from config
			cell.ColorNumber(cfg.NightColorNumber), // Night color from config
		),
		widgets.MaxWindow(time.Duration(cfg.MaxWindowZoom)*24*time.Hour), // Maximum zoom window from config
	)
}

// CreateTextWidget creates and configures the text display widget
func CreateTextWidget() (*text.Text, error) {
	return text.New(text.WrapAtWords())
}

// CreateSOTBarChart creates and configures the daily SOT bar chart widget
func CreateSOTBarChart() *widgets.SOTBarChart {
	return widgets.CreateSOTBarChart(
		widgets.SOTBarTitle("Daily Screen-On Time (7 days)"),
		widgets.SOTBarColors(
			cell.ColorCyan,   // Regular bar color
			cell.ColorYellow, // Today bar color
			cell.ColorWhite,  // Text color
		),
	)
}

// CreateUILayout creates the TUI container layout with all widgets
func CreateUILayout(t terminalapi.Terminal, chartWidget *widgets.BatteryChart, textWidget *text.Text, sotBarChart *widgets.SOTBarChart) (*container.Container, error) {
	return container.New(
		t,
		container.Border(linestyle.Light),
		container.BorderTitle("Battery Logger TUI - Tab/Shift+Tab: focus, q: quit, r: refresh"),
		container.KeyFocusNext(keyboard.KeyTab),
		container.KeyFocusPrevious(keyboard.KeyBacktab),
		container.SplitHorizontal(
			container.Top(
				container.ID("chart-container"),
				container.Border(linestyle.Light),
				container.BorderTitle("Battery % Over Time - i/o/mouse wheel: zoom, ←→: pan, esc: reset"),
				container.PlaceWidget(chartWidget),
			),
			container.Bottom(
				container.SplitVertical(
					container.Left(
						container.Border(linestyle.Light),
						container.BorderTitle("Battery Status & Prediction - ↑↓ to scroll"),
						container.PlaceWidget(textWidget),
					),
					container.Right(
						container.Border(linestyle.Light),
						container.BorderTitle("Daily Screen-On Time (7 days)"),
						container.PlaceWidget(sotBarChart),
					),
					container.SplitPercent(65), // Status text takes 65%, bar chart takes 35%
				),
			),
			container.SplitPercent(60),
		),
	)
}
