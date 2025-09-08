package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type PIDFile struct {
	Path string
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
	// If exists, check if process is alive; if not, remove and retry
	b, readErr := os.ReadFile(p.Path)
	if readErr != nil {
		return false, readErr
	}
	pid, _ := strconv.Atoi(string(b))
	if pid > 0 {
		if _, statErr := os.Stat(fmt.Sprintf("/proc/%d", pid)); statErr == nil {
			// Alive
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
