package submit

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// EnvInt reads name as an int, warning on a malformed value and falling back to
// def (a hidden misconfig should be loud). Shared by the step + the CLI for the
// KAGGLE_SUBMIT_MAX_ATTEMPTS knob.
func EnvInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	fmt.Fprintf(os.Stderr, "kaggle: ignoring malformed %s=%q, using %d\n", name, v, def)
	return def
}

// EnvDuration accepts a Go duration ("5s", "0") or a bare integer read as seconds.
// A malformed value warns (rather than silently defaulting — a hidden misconfig).
// Shared by the step + the CLI for the KAGGLE_SUBMIT_DELAY knob.
func EnvDuration(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	fmt.Fprintf(os.Stderr, "kaggle: ignoring malformed %s=%q, using %s\n", name, v, def)
	return def
}
