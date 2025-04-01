package cmdutil

import (
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"gotest.tools/v3/assert"
)

func contructUnknownNameError(level string) error {
	unknownErr := errors.New("unknown name")
	levelErr := fmt.Errorf("slog: level string %q: %w", level, unknownErr)
	return fmt.Errorf("unable to parse log level: %w", levelErr)
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name          string
		log           string
		expectedLevel slog.Level
		expectedErr   error
	}{
		{
			name:          "lowercase debug level",
			log:           "debug",
			expectedLevel: slog.LevelDebug,
			expectedErr:   nil,
		},
		{
			name:          "mixed case info level",
			log:           "InFo",
			expectedLevel: slog.LevelInfo,
			expectedErr:   nil,
		},
		{
			name:          "capitalized warn level",
			log:           "Warn",
			expectedLevel: slog.LevelWarn,
			expectedErr:   nil,
		},
		{
			name:          "mixed case error level",
			log:           "erROR",
			expectedLevel: slog.LevelError,
			expectedErr:   nil,
		},
		{
			name:          "capitalized debug level",
			log:           "Debug",
			expectedLevel: slog.LevelDebug,
			expectedErr:   nil,
		},
		{
			name:          "invalid level Test",
			log:           "Test",
			expectedLevel: slog.LevelInfo,
			expectedErr:   contructUnknownNameError("Test"),
		},
		{
			name:          "invalid level Test2",
			log:           "Test2",
			expectedLevel: slog.LevelInfo,
			expectedErr:   contructUnknownNameError("Test2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			logLevel, err := ParseLogLevel(tt.log)

			assert.Equal(t, tt.expectedLevel, logLevel)
			if err != nil {
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.Equal(t, tt.expectedErr, err)
			}
		})
	}
}
