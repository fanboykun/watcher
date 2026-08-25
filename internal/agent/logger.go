package agent

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

const LogFilename = "watcher.log"

// LogConfig controls the structured log level and on-disk retention.
// MaxSizeMB is the maximum size of the active log before it is rotated.
type LogConfig struct {
	Level      string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// Logger keeps the existing call-site API while delegating JSON encoding and
// attribute handling to log/slog.
type Logger struct {
	logger *slog.Logger
	closer io.Closer
}

func NewLogger(component string) *Logger {
	return NewLoggerWithWriter(component, os.Stdout, "info")
}

// NewLoggerWithWriter creates a JSON logger for a caller-provided destination.
func NewLoggerWithWriter(component string, out io.Writer, level string) *Logger {
	return newLogger(component, out, level, nil)
}

// NewFileLogger writes JSON logs to stdout and a rotating watcher.log file.
func NewFileLogger(component, logDir string, cfg LogConfig) (*Logger, error) {
	return newFileLogger(component, logDir, cfg, os.Stdout)
}

func newFileLogger(component, logDir string, cfg LogConfig, stdout io.Writer) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, LogFilename),
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}
	return newLogger(component, io.MultiWriter(stdout, rotator), cfg.Level, rotator), nil
}

func newLogger(component string, out io.Writer, level string, closer io.Closer) *Logger {
	parsedLevel := slog.LevelInfo
	if err := parsedLevel.UnmarshalText([]byte(strings.ToUpper(strings.TrimSpace(level)))); err != nil {
		parsedLevel = slog.LevelInfo
	}
	return &Logger{
		logger: slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: parsedLevel})).With("component", component),
		closer: closer,
	}
}

// WithComponent returns a logger sharing the output and level with a
// replacement component attribute, rather than nesting component values.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{logger: slog.New(l.logger.Handler()).With("component", component)}
}

func (l *Logger) Info(msg string, args ...any)  { l.logger.Info(msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.logger.Warn(msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.logger.Error(msg, args...) }
func (l *Logger) Debug(msg string, args ...any) { l.logger.Debug(msg, args...) }

// Close releases the rotating log file. It is safe to call on stdout-only and
// child loggers, which do not own a file destination.
func (l *Logger) Close() error {
	if l.closer == nil {
		return nil
	}
	return l.closer.Close()
}
