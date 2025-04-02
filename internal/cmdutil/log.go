package cmdutil

import (
	"fmt"
	"log/slog"
)

func ParseLogLevel(s string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return level, fmt.Errorf("unable to parse log level: %w", err)
	}

	return level, nil
}
