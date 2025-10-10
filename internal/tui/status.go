package tui

import (
	"sync"
	"time"

	"github.com/Prajwal-Prathiksh/battery-logger/internal/analytics"
)

// UIParams holds the real-time adjustable parameters
type UIParams struct {
	Refresh time.Duration
	mu      sync.RWMutex
}

// Get returns thread-safe copies of the parameters
func (p *UIParams) Get() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Refresh
}

// StatusInfo holds information needed for status display
type StatusInfo struct {
	Latest            analytics.Row
	TransitionTime    time.Time
	TransitionBatt    float64
	RateLabel         string
	SlopeStr          string
	Confidence        string
	Estimate          string
	EstimateDuration  time.Duration
	EstimateETA       time.Time
	TotalSamples      int
	ACSamples         int
	BattSamples       int
	TimeRange         time.Duration
	StartTime         string
	EndTime           string
	ConfigStr         string
	LogPath           string
	MaxChargePercent  int
	CycleCount        int
	HasCycleCount     bool
	ScreenOnTime      analytics.ScreenOnTimeResult
	TodayScreenOnTime analytics.ScreenOnTimeResult
	LastSuspendEvent  *analytics.SuspendEvent
}
