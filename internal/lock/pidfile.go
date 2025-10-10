package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PIDFile struct {
	Path string
}

// isBatteryZenProcess checks if the given PID belongs to a battery-zen process
func isBatteryZenProcess(pid int) bool {
	// Check if the process exists
	procPath := fmt.Sprintf("/proc/%d", pid)
	if _, err := os.Stat(procPath); err != nil {
		return false
	}

	// Read the process command name
	commPath := fmt.Sprintf("/proc/%d/comm", pid)
	comm, err := os.ReadFile(commPath)
	if err != nil {
		return false
	}

	// Check if it's battery-zen (trim newline)
	processName := strings.TrimSpace(string(comm))
	if processName == "battery-zen" {
		return true
	}

	// Also check cmdline as a fallback (in case the process name is truncated)
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	cmdline, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}

	// cmdline is null-separated, so convert nulls to spaces and check
	cmdlineStr := string(cmdline)
	cmdlineStr = strings.ReplaceAll(cmdlineStr, "\x00", " ")
	return strings.Contains(cmdlineStr, "battery-zen")
}

func (p *PIDFile) Acquire() (bool, error) {
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
		return false, err
	}
	// Try O_EXCL
	f, err := os.OpenFile(p.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		defer f.Close()
		_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
		return true, nil
	}
	// If exists, check if process is alive and is actually battery-zen; if not, remove and retry
	b, readErr := os.ReadFile(p.Path)
	if readErr != nil {
		return false, readErr
	}
	pid, _ := strconv.Atoi(string(b))
	if pid > 0 {
		if isBatteryZenProcess(pid) {
			// Another battery-zen instance is actually running
			return false, nil
		}
	}
	_ = os.Remove(p.Path)
	f, err = os.OpenFile(p.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	return true, nil
}

func (p *PIDFile) Release() { _ = os.Remove(p.Path) }
