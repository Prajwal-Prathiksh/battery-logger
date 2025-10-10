package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/widgets/text"
)

// LineSpec holds formatting information for status text display
type LineSpec struct {
	Text     string
	Color    cell.Color
	UseColor bool
}

// BuildStatusLines centralizes ALL string construction & styling.
func BuildStatusLines(info StatusInfo) []LineSpec {
	var lines []LineSpec

	appendLine := func(txt string, color cell.Color, useColor bool) {
		// Use strings package meaningfully to normalize whitespace.
		txt = strings.TrimSpace(txt)
		lines = append(lines, LineSpec{Text: txt, Color: color, UseColor: useColor})
	}

	// Header: AC status
	acStatus := "Unplugged"
	acIcon := "󱐤"
	if info.Latest.AC {
		acStatus = "Plugged In"
		acIcon = ""
	}
	appendLine(fmt.Sprintf("%s  AC Status: %s", acIcon, acStatus), cell.ColorYellow, true)

	// Delta since last transition
	if !info.TransitionTime.IsZero() {
		durationSince := info.Latest.T.Sub(info.TransitionTime).Round(time.Minute)
		if info.Latest.AC {
			battGain := info.Latest.Batt - info.TransitionBatt
			appendLine(
				fmt.Sprintf("--    Plugged in for %s, battery ↑ %.1f%% (start: %.1f%%)",
					FormatDurationAuto(durationSince), battGain, info.TransitionBatt),
				0, false,
			)
		} else {
			battDrop := info.TransitionBatt - info.Latest.Batt
			appendLine(
				fmt.Sprintf("--    On battery for %s (since: %s), battery ↓ %.1f%% (start: %.1f%%)",
					FormatDurationAuto(durationSince), info.TransitionTime.Format("Jan 2 15:04"), battDrop, info.TransitionBatt),
				0, false,
			)
		}
	}
	if info.Latest.AC {
		// If we have an estimate duration, also show the ETA (by: time)
		if info.EstimateDuration > 0 {
			appendLine(fmt.Sprintf("--    Time to Full (%d%%): %s (by: %s)", info.MaxChargePercent, info.Estimate, info.EstimateETA.Format("15:04")), 0, false)
		} else {
			appendLine(fmt.Sprintf("--    Time to Full (%d%%): %s", info.MaxChargePercent, info.Estimate), 0, false)
		}
	} else {
		if info.EstimateDuration > 0 {
			appendLine(fmt.Sprintf("--    Time to Empty (0%%): %s (by: %s)", info.Estimate, info.EstimateETA.Format("15:04")), 0, false)
		} else {
			appendLine(fmt.Sprintf("--    Time to Empty (0%%): %s", info.Estimate), 0, false)
		}
	}

	// Spacer
	appendLine("", 0, false)

	// Battery status section
	appendLine("󰤁  Battery Status:", 0, false)
	// Current battery & cycles
	appendLine(fmt.Sprintf("--    Current Battery: %.1f%%", info.Latest.Batt), 0, false)
	if info.HasCycleCount {
		appendLine(fmt.Sprintf("--    Battery Cycles: %d", info.CycleCount), 0, false)
	}

	// Rate + estimate
	appendLine(fmt.Sprintf("--    %s: %s %s", info.RateLabel, info.SlopeStr, info.Confidence), 0, false)

	// Spacer
	appendLine("", 0, false)

	// Screen-on time section
	appendLine("  Screen-On Time (SOT):", cell.ColorCyan, true)

	// Current session (since last suspend/wake)
	if info.ScreenOnTime.LastActiveSession > 0 {
		appendLine(fmt.Sprintf("--    Current session: %s", FormatDurationAuto(info.ScreenOnTime.LastActiveSession)), 0, false)
	}

	// Today's total SOT
	if info.TodayScreenOnTime.TotalActiveTime > 0 {
		appendLine(fmt.Sprintf("--    Today's total: %s", FormatDurationAuto(info.TodayScreenOnTime.TotalActiveTime)), 0, false)
		if len(info.TodayScreenOnTime.SuspendEvents) > 0 {
			appendLine(fmt.Sprintf("--    Today's suspends: %d (total: %s)",
				len(info.TodayScreenOnTime.SuspendEvents),
				FormatDurationAuto(info.TodayScreenOnTime.SuspendTime)), 0, false)
		}
	}

	// Last suspend/shutdown event details
	if info.LastSuspendEvent != nil {
		appendLine(fmt.Sprintf("--    Last suspend: %s ago (lasted %s)",
			FormatDurationAuto(time.Since(info.LastSuspendEvent.EndTime)),
			FormatDurationAuto(info.LastSuspendEvent.Duration)), 0, false)
		if info.LastSuspendEvent.BatteryDrop > 0 {
			appendLine(fmt.Sprintf("--    Battery drain during suspend: %.1f%% (%.1f%% → %.1f%%)",
				info.LastSuspendEvent.BatteryDrop,
				info.LastSuspendEvent.BatteryBefore,
				info.LastSuspendEvent.BatteryAfter), cell.ColorRed, true)
		} else if info.LastSuspendEvent.BatteryDrop < 0 {
			appendLine(fmt.Sprintf("--    Battery gained during suspend: +%.1f%% (%.1f%% → %.1f%%)",
				-info.LastSuspendEvent.BatteryDrop,
				info.LastSuspendEvent.BatteryBefore,
				info.LastSuspendEvent.BatteryAfter), cell.ColorGreen, true)
		}
	}

	// Total active time in the data window
	if info.ScreenOnTime.TotalActiveTime > 0 {
		appendLine(fmt.Sprintf("--    Total active time in window: %s", FormatDurationAuto(info.ScreenOnTime.TotalActiveTime)), 0, false)
		if len(info.ScreenOnTime.SuspendEvents) > 0 {
			appendLine(fmt.Sprintf("--    Total suspend events: %d (total: %s)",
				len(info.ScreenOnTime.SuspendEvents),
				FormatDurationAuto(info.ScreenOnTime.SuspendTime)), 0, false)
		}
	}

	// Spacer
	appendLine("", 0, false)

	// Summary section
	appendLine("  Data Summary:", 0, false)
	appendLine(fmt.Sprintf("--    Total samples: %d (spanning %s)", info.TotalSamples, FormatDurationAuto(info.TimeRange.Round(time.Minute))), 0, false)
	appendLine(fmt.Sprintf("--    AC plugged: %d samples", info.ACSamples), cell.ColorGreen, true)
	appendLine(fmt.Sprintf("--    On battery: %d samples", info.BattSamples), cell.ColorRed, true)
	appendLine(fmt.Sprintf("--    Time range: %s to %s", info.StartTime, info.EndTime), 0, false)

	// Spacer
	appendLine("", 0, false)

	// Paths & config
	appendLine(fmt.Sprintf("  Data file: %s", info.LogPath), 0, false)
	appendLine(info.ConfigStr, 0, false)

	return lines
}

// UpdateStatusText writes formatted status information to the text widget
func UpdateStatusText(textWidget *text.Text, info StatusInfo) {
	textWidget.Reset()
	for _, ln := range BuildStatusLines(info) {
		if ln.UseColor {
			textWidget.Write(ln.Text+"\n", text.WriteCellOpts(cell.FgColor(ln.Color)))
		} else {
			textWidget.Write(ln.Text + "\n")
		}
	}
}
