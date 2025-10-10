package tui

import (
	"fmt"
	"time"
)

// FormatDurationAuto formats a duration as HH:mm if <24h, or as Xd Yh if >=24h
func FormatDurationAuto(dur time.Duration) string {
	if dur < 24*time.Hour {
		h := int(dur.Hours())
		m := int(dur.Minutes()) % 60
		return fmt.Sprintf("%02dh %02dm", h, m)
	} else {
		days := int(dur.Hours()) / 24
		hours := int(dur.Hours()) % 24
		return fmt.Sprintf("%dd %dh", days, hours)
	}
}
