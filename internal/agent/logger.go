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
	logger      *slog.Logger
	handler     slog.Handler
	component   string
	watcherID   uint
	watcherName string
	trace       Trace
	closer      io.Closer
}

// NewLogger creates a configured logger.
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

// newFileLogger creates a logger that writes JSON records to rotating storage and stdout.
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
	writer := &scopedLogWriter{global: io.MultiWriter(stdout, rotator), globalFile: rotator, logDir: logDir, cfg: cfg, files: make(map[uint]*lumberjack.Logger)}
	return newLogger(component, writer, cfg.Level, writer), nil
}

// newLogger creates the shared slog wrapper and records ownership of its output closer.
func newLogger(component string, out io.Writer, level string, closer io.Closer) *Logger {
	parsedLevel := slog.LevelInfo
	if err := parsedLevel.UnmarshalText([]byte(strings.ToUpper(strings.TrimSpace(level)))); err != nil {
		parsedLevel = slog.LevelInfo
	}
	log := &Logger{handler: slog.NewJSONHandler(out, &slog.HandlerOptions{Level: parsedLevel}), component: component, closer: closer}
	log.rebuild()
	return log
}

// WithComponent replaces the component while retaining watcher identity and destinations.
func (l *Logger) WithComponent(component string) *Logger {
	child := *l
	child.component, child.closer = component, nil
	child.rebuild()
	return &child
}

// WithWatcher routes records to the watcher ID's file as well as the global output.
func (l *Logger) WithWatcher(id uint, name string) *Logger {
	child := *l
	child.watcherID, child.watcherName, child.component, child.closer = id, name, name, nil
	child.rebuild()
	return &child
}

func (l *Logger) rebuild() {
	l.logger = slog.New(l.handler).With("component", l.component)
	if l.watcherID != 0 {
		l.logger = l.logger.With("watcher_id", l.watcherID, "watcher_name", l.watcherName)
	}
	if l.trace.CorrelationID != "" {
		l.logger = l.logger.With("correlation_id", l.trace.CorrelationID)
	}
	if l.trace.RequestID != "" {
		l.logger = l.logger.With("request_id", l.trace.RequestID)
	}
	if l.trace.PollID != "" {
		l.logger = l.logger.With("poll_id", l.trace.PollID)
	}
	if l.trace.TriggeredBy != "" {
		l.logger = l.logger.With("triggered_by", l.trace.TriggeredBy)
	}
}

// Info writes an informational structured log entry.
func (l *Logger) Info(msg string, args ...any) { l.logger.Info(msg, args...) }

// Warn writes a warning structured log entry.
func (l *Logger) Warn(msg string, args ...any) { l.logger.Warn(msg, args...) }

// Error writes an error-level structured log entry.
func (l *Logger) Error(msg string, args ...any) { l.logger.Error(msg, args...) }

// Debug writes a debug structured log entry.
func (l *Logger) Debug(msg string, args ...any) { l.logger.Debug(msg, args...) }

// Close releases the rotating log file. It is safe to call on stdout-only and
// child loggers, which do not own a file destination.
func (l *Logger) Close() error {
	if l.closer == nil {
		return nil
	}
	return l.closer.Close()
}
