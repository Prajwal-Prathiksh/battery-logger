package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	IntervalSecs     int    `toml:"interval_secs"`
	IntervalSecsOnAC int    `toml:"interval_secs_on_ac"`
	Timezone         string `toml:"timezone"` // "UTC" or "Local"
	LogDir           string `toml:"log_dir"`
	LogFile          string `toml:"log_file"`
	MaxLines         int    `toml:"max_lines"`
	TrimBuffer       int    `toml:"trim_buffer"`
	MaxChargePercent int    `toml:"max_charge_percent"`
}

func Defaults() Config {
	return Config{
		IntervalSecs:     60,
		IntervalSecsOnAC: 300,
		Timezone:         "Local",
		LogDir:           filepath.Join(xdgStateHome(), "battery-logger"),
		LogFile:          "battery.csv",
		MaxLines:         1000,
		TrimBuffer:       100,
		MaxChargePercent: 100,
	}
}

// getConfigPathsInternal returns the list of config file paths that are checked
func getConfigPathsInternal() []string {
	return []string{
		// Local project config
		filepath.Join("internal", "config", "config.toml"),
		// User config
		filepath.Join(xdgConfigHome(), "battery-logger", "config.toml"),
		// System config
		"/etc/battery-logger/config.toml",
	}
}

// GetConfigPaths returns the list of config file paths that are checked, and which ones exist
func GetConfigPaths() ([]string, []string) {
	relativePaths := getConfigPathsInternal()

	var allPaths []string
	var existingPaths []string

	for _, path := range relativePaths {
		// Resolve to absolute path
		absPath, err := filepath.Abs(path)
		if err != nil {
			// If we can't resolve to absolute, use the original path
			absPath = path
		}
		allPaths = append(allPaths, absPath)

		if _, err := os.Stat(path); err == nil {
			existingPaths = append(existingPaths, absPath)
		}
	}

	return allPaths, existingPaths
}

func Load() (Config, error) {
	cfg := Defaults()

	// Get config paths from the shared function
	configPaths := getConfigPathsInternal()

	// Load configs in order, later ones override earlier ones
	for _, path := range configPaths {
		if err := loadConfigFile(path, &cfg); err != nil {
			// Only return error if it's not a "file not found" error
			if !errors.Is(err, os.ErrNotExist) {
				return cfg, err
			}
		}
	}

	// Expand ~ in LogDir
	if strings.HasPrefix(cfg.LogDir, "~") {
		home, _ := os.UserHomeDir()
		cfg.LogDir = filepath.Join(home, strings.TrimPrefix(cfg.LogDir, "~"))
	}
	return cfg, nil
}

func loadConfigFile(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key = value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes from string values
		if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			value = strings.Trim(value, `"`)
		}

		// Set config values based on key
		switch key {
		case "interval_secs":
			if val, err := strconv.Atoi(value); err == nil {
				cfg.IntervalSecs = val
			}
		case "interval_secs_on_ac":
			if val, err := strconv.Atoi(value); err == nil {
				cfg.IntervalSecsOnAC = val
			}
		case "timezone":
			cfg.Timezone = value
		case "log_dir":
			cfg.LogDir = value
		case "log_file":
			cfg.LogFile = value
		case "max_lines":
			if val, err := strconv.Atoi(value); err == nil {
				cfg.MaxLines = val
			}
		case "trim_buffer":
			if val, err := strconv.Atoi(value); err == nil {
				cfg.TrimBuffer = val
			}
		case "max_charge_percent":
			if val, err := strconv.Atoi(value); err == nil {
				cfg.MaxChargePercent = val
			}
		}
	}

	return scanner.Err()
}

func XDGLogPath(cfg Config) (string, error) {
	if _, err := os.Stat(cfg.LogDir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
			return "", err
		}
	}
	return filepath.Join(cfg.LogDir, cfg.LogFile), nil
}

func Now(cfg Config) time.Time {
	if strings.EqualFold(cfg.Timezone, "Local") {
		return time.Now()
	}
	return time.Now().UTC()
}

func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func xdgStateHome() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state")
}
