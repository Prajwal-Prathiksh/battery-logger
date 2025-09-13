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

// FilterWindow returns a slice of rows containing only those entries
// that fall within the specified time duration before the last entry.
func FilterWindow(rows []Row, since time.Duration) []Row {
	if len(rows) == 0 {
		return nil
	}
	cut := rows[len(rows)-1].T.Add(-since)
	i := 0
	for i < len(rows) && rows[i].T.Before(cut) {
		i++
	}
	return rows[i:]
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
// For charging (AC=true): returns positive rate and time to 100%.
// For discharging (AC=false): returns negative rate and time to 0%.
// Returns rate (% per minute), estimate (minutes), confidence string, and success flag.
func CalculateRateAndEstimate(rows []Row, currentBatt float64, alpha float64) (float64, float64, string, bool) {
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
			estimate = (100 - currentBatt) / rate // Time to reach 100%
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

	// Find columns by header name (flexible: case-insensitive, allow spaces)
	col := func(name string) int {
		name = strings.ToLower(strings.TrimSpace(name))
		for i, h := range rows[0] {
			if strings.ToLower(strings.TrimSpace(h)) == name {
				return i
			}
		}
		return -1
	}

	tsIdx := col("timestamp")
	acIdx := col("ac_connected")
	if acIdx == -1 {
		acIdx = col("ac")
	}
	if acIdx == -1 {
		acIdx = col("ac plugged in (bool)")
	}
	if acIdx == -1 {
		acIdx = col("ac plugged in")
	}
	battIdx := col("battery_life")
	if battIdx == -1 {
		battIdx = col("battery")
	}
	if battIdx == -1 {
		battIdx = col("battery life (%)")
	}

	if tsIdx == -1 || acIdx == -1 || battIdx == -1 {
		return nil, fmt.Errorf("expected headers: timestamp, ac_connected, battery_life (or similar)")
	}

	var out []Row
	for i := 1; i < len(rows); i++ {
		rec := rows[i]
		if len(rec) <= battIdx || len(rec) <= tsIdx || len(rec) <= acIdx {
			continue
		}

		t, err := time.Parse(time.RFC3339, strings.TrimSpace(rec[tsIdx]))
		if err != nil {
			// Try some common fallback formats if needed
			layouts := []string{
				"2006-01-02 15:04:05",
				"2006-01-02 15:04:05 -0700",
				"2006-01-02T15:04:05",
			}
			ok := false
			for _, lay := range layouts {
				if tt, e2 := time.Parse(lay, strings.TrimSpace(rec[tsIdx])); e2 == nil {
					t = tt
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}

		ac, err := ParseBoolLoose(rec[acIdx])
		if err != nil {
			continue
		}

		b, err := strconv.ParseFloat(strings.TrimSpace(rec[battIdx]), 64)
		if err != nil {
			continue
		}

		out = append(out, Row{T: t, AC: ac, Batt: b})
	}
	return out, nil
}
