package log

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

var logFileHandler *os.File

func FlushLog() {
	if logFileHandler != nil {
		logFileHandler.Sync()
	}
}

func FlushAndClose() error {
	if logFileHandler != nil {
		logFileHandler.Sync()
		return logFileHandler.Close()
	}

	return nil
}

func NewLogger(debug bool, logDir, supervisor string) (*slog.Logger, error) {
	return NewLoggerWithCallback(debug, logDir, supervisor, nil)
}

// NewLoggerWithCallback creates a logger with an optional callback for log entries
func NewLoggerWithCallback(debug bool, logDir, supervisor string, callback func(LogEntry)) (*slog.Logger, error) {
	if logDir == "" {
		logDir = "logs"
	}

	if _, err := os.Stat(logDir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(logDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("error creating log directory: %w", err)
		}
	}

	fileName := "Koolo-log-" + time.Now().Format("2006-01-02-15-04-05") + ".txt"
	source := "koolo"
	if supervisor != "" {
		fileName = fmt.Sprintf("Supervisor-log-%s-%s.txt", supervisor, time.Now().Format("2006-01-02-15-04-05"))
		source = supervisor
	}

	lfh, err := os.Create(logDir + "/" + fileName)
	if err != nil {
		return nil, err
	}

	// Close previous log file handler if exists to prevent leak
	if logFileHandler != nil {
		logFileHandler.Sync()
		logFileHandler.Close()
	}
	logFileHandler = lfh

	level := slog.LevelDebug
	if !debug {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key != slog.TimeKey {
				return a
			}

			t := a.Value.Time()
			a.Value = slog.StringValue(t.Format(time.TimeOnly))

			return a
		},
	}

	var handler slog.Handler
	handler = slog.NewTextHandler(io.MultiWriter(logFileHandler, os.Stdout), opts)

	// Wrap with buffer handler if callback is provided
	if callback != nil {
		handler = NewBufferHandler(handler, source, callback)
	}

	return slog.New(handler), nil
}
