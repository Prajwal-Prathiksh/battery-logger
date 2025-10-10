package analytics

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Row represents a single CSV record
type Row struct {
	T    time.Time
	AC   bool
	Batt float64
}

// ParseBoolLoose parses boolean values in various formats including
// "true"/"false", "t"/"f", "1"/"0", "yes"/"no", "y"/"n", and integers
// where 0 is false and any other value is true.
func ParseBoolLoose(s string) (bool, error) {
	ss := strings.TrimSpace(strings.ToLower(s))
	switch ss {
	case "true", "t", "1", "yes", "y":
		return true, nil
	case "false", "f", "0", "no", "n":
		return false, nil
	default:
		// Try parsing as integer (0 = false, anything else = true)
		if val, err := strconv.Atoi(ss); err == nil {
			return val != 0, nil
		}
		return false, fmt.Errorf("bad bool: %q", s)
	}
}

// WeightedLinReg performs weighted linear regression on battery data
// using exponential weights (more recent data has higher weight).
// x represents minutes relative to the last point (<=0), weights w = exp(alpha*x).
// Returns slope b (% per minute), intercept a (% at x=0 i.e., "now"), and success flag.
func WeightedLinReg(rows []Row, alpha float64) (float64, float64, bool) {
	if len(rows) < 2 {
		return 0, 0, false
	}
	tNow := rows[len(rows)-1].T

	var sumW, sumWX, sumWY, sumWXX, sumWXY float64
	for _, r := range rows {
		x := r.T.Sub(tNow).Minutes() // <= 0
		w := math.Exp(alpha * x)     // more recent -> larger weight
		y := r.Batt
		sumW += w
		sumWX += w * x
		sumWY += w * y
		sumWXX += w * x * x
		sumWXY += w * x * y
	}

	den := sumW*sumWXX - sumWX*sumWX
	if den == 0 {
		return 0, 0, false
	}
	b := (sumW*sumWXY - sumWX*sumWY) / den
	a := (sumWY - b*sumWX) / sumW
	return b, a, true
}

// FmtDur formats a duration in minutes into a human-readable string
// in the format "Xh Ym". Returns "—" for invalid values (NaN, Inf, or negative).
func FmtDur(mins float64) string {
	if math.IsNaN(mins) || math.IsInf(mins, 0) || mins < 0 {
		return "—"
	}
	d := time.Duration(mins * float64(time.Minute))
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	return fmt.Sprintf("%dh %dm", h, m)
}

// FilterContiguousACState filters rows to include only the most recent contiguous
// samples with the specified AC state (true for plugged, false for unplugged).
// Returns rows in chronological order (oldest first).
func FilterContiguousACState(rows []Row, acState bool) []Row {
	if len(rows) == 0 {
		return nil
	}

	var filtered []Row
	// Walk backwards from the end to find contiguous samples with the specified AC state
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].AC == acState {
			filtered = append([]Row{rows[i]}, filtered...)
		} else {
			break
		}
	}
	return filtered
}

// CalculateRateAndEstimate calculates the battery rate and time estimate based on AC state.
// For charging (AC=true): returns positive rate and time to maxChargePercent.
// For discharging (AC=false): returns negative rate and time to 0%.
// Returns rate (% per minute), estimate (minutes), confidence string, and success flag.
func CalculateRateAndEstimate(rows []Row, currentBatt float64, alpha float64, maxChargePercent int) (float64, float64, string, bool) {
	if len(rows) < 2 {
		return 0, 0, "(need ≥2 samples with same AC state)", false
	}

	// Determine if we're looking at charging or discharging data
	isCharging := rows[0].AC // All rows should have the same AC state due to filtering

	rate, _, ok := WeightedLinReg(rows, alpha)
	if !ok {
		return 0, 0, "(regression failed)", false
	}

	var estimate float64
	var confidence string

	if isCharging {
		// When charging, rate should be positive (battery % increasing)
		if rate > 1e-6 { // Positive rate means charging
			estimate = (float64(maxChargePercent) - currentBatt) / rate // Time to reach max charge
			confidence = fmt.Sprintf("(based on %d charging samples)", len(rows))
		} else {
			// Rate is negative or zero while plugged in - not actually charging
			estimate = math.Inf(1) // Infinite time (already at max or discharging while plugged)
			confidence = "(not charging or already full)"
		}
	} else {
		// When discharging, rate should be negative (battery % decreasing)
		if rate < -1e-6 { // Negative rate means discharging
			estimate = -currentBatt / rate // Time to reach 0%
			confidence = fmt.Sprintf("(based on %d discharging samples)", len(rows))
		} else {
			// Rate is positive or zero while unplugged - unusual
			estimate = math.Inf(1) // Infinite time (not actually discharging)
			confidence = "(not discharging)"
		}
	}

	return rate, estimate, confidence, true
}

// ParseCSVRows parses CSV data with flexible column detection and
// converts it to a slice of Row structs. The CSV must contain
// timestamp, AC connection status, and battery percentage columns.
// Column names are matched case-insensitively with various aliases supported.
func ParseCSVRows(rows [][]string) ([]Row, error) {
	if len(rows) == 0 {
		return nil, errors.New("empty csv")
	}

	tsIdx, acIdx, battIdx, err := findColumns(rows[0])
	if err != nil {
		return nil, err
	}

	var out []Row
	for i := 1; i < len(rows); i++ {
		row, err := parseCSVRow(rows[i], tsIdx, acIdx, battIdx)
		if err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func findColumns(header []string) (tsIdx, acIdx, battIdx int, err error) {
	col := func(name string) int {
		name = strings.ToLower(strings.TrimSpace(name))
		for i, h := range header {
			if strings.ToLower(strings.TrimSpace(h)) == name {
				return i
			}
		}
		return -1
	}

	tsIdx = col("timestamp")
	acIdx = col("ac_connected")
	if acIdx == -1 {
		acIdx = col("ac")
	}
	if acIdx == -1 {
		acIdx = col("ac plugged in (bool)")
	}
	if acIdx == -1 {
		acIdx = col("ac plugged in")
	}
	battIdx = col("battery_life")
	if battIdx == -1 {
		battIdx = col("battery")
	}
	if battIdx == -1 {
		battIdx = col("battery life (%)")
	}

	if tsIdx == -1 || acIdx == -1 || battIdx == -1 {
		return -1, -1, -1, fmt.Errorf("expected headers: timestamp, ac_connected, battery_life (or similar)")
	}
	return tsIdx, acIdx, battIdx, nil
}

func parseCSVRow(rec []string, tsIdx, acIdx, battIdx int) (Row, error) {
	if len(rec) <= battIdx || len(rec) <= tsIdx || len(rec) <= acIdx {
		return Row{}, fmt.Errorf("insufficient columns")
	}

	t, err := parseTimestamp(strings.TrimSpace(rec[tsIdx]))
	if err != nil {
		return Row{}, err
	}

	ac, err := ParseBoolLoose(rec[acIdx])
	if err != nil {
		return Row{}, err
	}

	b, err := strconv.ParseFloat(strings.TrimSpace(rec[battIdx]), 64)
	if err != nil {
		return Row{}, err
	}

	return Row{T: t, AC: ac, Batt: b}, nil
}

func parseTimestamp(tsStr string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, tsStr)
	if err == nil {
		return t, nil
	}

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02T15:04:05",
	}
	for _, lay := range layouts {
		if tt, e2 := time.Parse(lay, tsStr); e2 == nil {
			return tt, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse timestamp")
}

// SuspendEvent represents a detected suspend/shutdown period
type SuspendEvent struct {
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	BatteryBefore float64
	BatteryAfter  float64
	BatteryDrop   float64
}

// DetectSuspendEvents identifies periods where data logging was interrupted,
// indicating system suspend or shutdown. Returns events in chronological order.
func DetectSuspendEvents(rows []Row, gapThresholdMinutes int) []SuspendEvent {
	if len(rows) < 2 {
		return nil
	}

	var events []SuspendEvent
	threshold := time.Duration(gapThresholdMinutes) * time.Minute

	for i := 1; i < len(rows); i++ {
		gap := rows[i].T.Sub(rows[i-1].T)
		if gap >= threshold {
			event := SuspendEvent{
				StartTime:     rows[i-1].T,
				EndTime:       rows[i].T,
				Duration:      gap,
				BatteryBefore: rows[i-1].Batt,
				BatteryAfter:  rows[i].Batt,
				BatteryDrop:   rows[i-1].Batt - rows[i].Batt,
			}
			events = append(events, event)
		}
	}

	return events
}

// ScreenOnTimeResult holds screen-on time calculation results
type ScreenOnTimeResult struct {
	TotalActiveTime   time.Duration  // Total time with active data points
	SuspendTime       time.Duration  // Total time in suspend/shutdown
	LastActiveSession time.Duration  // Active time since last suspend/wake
	SuspendEvents     []SuspendEvent // All suspend events in the period
}

// CalculateScreenOnTime calculates screen-on time by detecting gaps in data logging.
// Active time = total time span - suspend time (gaps >= threshold).
// This is a proxy for screen-on time since logging typically happens when system is active.
func CalculateScreenOnTime(rows []Row, gapThresholdMinutes int) ScreenOnTimeResult {
	result := ScreenOnTimeResult{}

	if len(rows) < 2 {
		return result
	}

	// Detect all suspend events
	result.SuspendEvents = DetectSuspendEvents(rows, gapThresholdMinutes)

	// Calculate total suspend time
	for _, event := range result.SuspendEvents {
		result.SuspendTime += event.Duration
	}

	// Total time span
	totalTimeSpan := rows[len(rows)-1].T.Sub(rows[0].T)

	// Active time = total span - suspend time
	result.TotalActiveTime = totalTimeSpan - result.SuspendTime

	// Calculate time since last suspend/wake (current active session)
	if len(result.SuspendEvents) > 0 {
		lastSuspendEnd := result.SuspendEvents[len(result.SuspendEvents)-1].EndTime
		result.LastActiveSession = rows[len(rows)-1].T.Sub(lastSuspendEnd)
	} else {
		// No suspends detected, entire period is one session
		result.LastActiveSession = result.TotalActiveTime
	}

	return result
}

// CalculateDailyScreenOnTime calculates screen-on time for a specific day.
// Returns active time and suspend events for that day only.
func CalculateDailyScreenOnTime(rows []Row, targetDate time.Time, gapThresholdMinutes int) ScreenOnTimeResult {
	// Filter rows to only include the target date
	startOfDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, targetDate.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var dayRows []Row
	for _, row := range rows {
		if row.T.After(startOfDay) && row.T.Before(endOfDay) {
			dayRows = append(dayRows, row)
		}
	}

	if len(dayRows) == 0 {
		return ScreenOnTimeResult{}
	}

	return CalculateScreenOnTime(dayRows, gapThresholdMinutes)
}
