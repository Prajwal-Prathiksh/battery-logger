package tui

import (
	"fmt"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/config"
	"github.com/Prajwal-Prathiksh/battery-logger/internal/sysfs"
)

// FindLastACTransition finds when the current AC status started
func FindLastACTransition(rows []analytics.Row) (time.Time, float64) {
	if len(rows) == 0 {
		return time.Time{}, 0
	}

	currentACState := rows[len(rows)-1].AC

	// Walk backwards to find when this AC state started
	for i := len(rows) - 2; i >= 0; i-- {
		if rows[i].AC != currentACState {
			// Found the transition point
			return rows[i+1].T, rows[i+1].Batt
		}
	}

	// No transition found, current state exists from the beginning
	return rows[0].T, rows[0].Batt
}

// GenerateStatusInfo processes battery data to create status information (logic only)
func GenerateStatusInfo(rows []analytics.Row, alpha float64, uiParams *UIParams, logPath string, cfg config.Config) StatusInfo {
	latest := rows[len(rows)-1]

	// Find when the current AC status started
	transitionTime, transitionBatt := FindLastACTransition(rows)

	// For regression, consider only the most recent contiguous samples with the same AC state
	currentACState := latest.AC
	contiguousSamples := analytics.FilterContiguousACState(rows, currentACState)

	var est string
	var slopeStr string
	var confidence string
	var rateLabel string
	var estimateDuration time.Duration
	var estimateETA time.Time

	if len(contiguousSamples) >= 2 {
		rate, estimateMins, conf, ok := analytics.CalculateRateAndEstimate(contiguousSamples, latest.Batt, alpha, cfg.MaxChargePercent)
		confidence = conf
		if ok {
			if currentACState {
				rateLabel = "Charge Rate"
			} else {
				rateLabel = "Discharge Rate"
			}
			dur := time.Duration(estimateMins * float64(time.Minute)).Round(time.Minute)
			est = FormatDurationAuto(dur)
			estimateDuration = dur
			estimateETA = time.Now().Add(dur).Round(time.Minute)
			slopeStr = fmt.Sprintf("%.3f %%/min", rate)
		} else {
			if currentACState {
				rateLabel = "Charge Rate"
			} else {
				rateLabel = "Discharge Rate"
			}
			est = "—"
			slopeStr = "n/a"
		}
	} else {
		if currentACState {
			rateLabel = "Charge Rate"
		} else {
			rateLabel = "Discharge Rate"
		}
		est = "—"
		slopeStr = "n/a"
		acStateStr := "charging"
		if !currentACState {
			acStateStr = "discharging"
		}
		confidence = fmt.Sprintf("(need ≥2 %s samples)", acStateStr)
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
	startTime := rows[0].T.Format("Jan 2 15:04")
	endTime := rows[len(rows)-1].T.Format("Jan 2 15:04")

	// Get config file paths
	_, existingConfigPaths := config.GetConfigPaths()
	var configStr string
	if len(existingConfigPaths) == 0 {
		configStr = "  Config: Using defaults (no config file found)" // nf-md-cog
	} else if len(existingConfigPaths) == 1 {
		configStr = fmt.Sprintf("  Config file: %s", existingConfigPaths[0]) // nf-md-cog
	} else {
		configStr = fmt.Sprintf("  Config files: %s (+ %d more)", existingConfigPaths[len(existingConfigPaths)-1], len(existingConfigPaths)-1) // nf-md-cog
	}

	// Get battery cycle count
	cycleCount, hasCycleCount := sysfs.BatteryCycleCount()

	// Calculate screen-on time and suspend events
	screenOnTime := analytics.CalculateScreenOnTime(rows, cfg.SuspendGapMinutes)

	// Calculate today's screen-on time
	now := time.Now()
	todayScreenOnTime := analytics.CalculateDailyScreenOnTime(rows, now, cfg.SuspendGapMinutes)

	// Get the most recent suspend event
	var lastSuspendEvent *analytics.SuspendEvent
	if len(screenOnTime.SuspendEvents) > 0 {
		lastSuspendEvent = &screenOnTime.SuspendEvents[len(screenOnTime.SuspendEvents)-1]
	}

	return StatusInfo{
		Latest:            latest,
		TransitionTime:    transitionTime,
		TransitionBatt:    transitionBatt,
		RateLabel:         rateLabel,
		SlopeStr:          slopeStr,
		Confidence:        confidence,
		Estimate:          est,
		EstimateDuration:  estimateDuration,
		EstimateETA:       estimateETA,
		TotalSamples:      totalSamples,
		ACSamples:         acSamples,
		BattSamples:       battSamples,
		TimeRange:         timeRange,
		StartTime:         startTime,
		EndTime:           endTime,
		ConfigStr:         configStr,
		LogPath:           logPath,
		MaxChargePercent:  cfg.MaxChargePercent,
		CycleCount:        cycleCount,
		HasCycleCount:     hasCycleCount,
		ScreenOnTime:      screenOnTime,
		TodayScreenOnTime: todayScreenOnTime,
		LastSuspendEvent:  lastSuspendEvent,
	}
}
