package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Logger struct {
	component string
	out       io.Writer
}

type logEntry struct {
	Time      string         `json:"time"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Msg       string         `json:"msg"`
	Fields    map[string]any `json:"fields,omitempty"`
}

func NewLogger(component string) *Logger {
	return &Logger{component: component, out: os.Stdout}
}

func NewFileLogger(component, logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	path := filepath.Join(logDir, "watcher.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &Logger{
		component: component,
		out:       io.MultiWriter(os.Stdout, f),
	}, nil
}

// WithComponent returns a child logger with a different component label.
// Used so each RepoWatcher logs with its own name as context.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{component: component, out: l.out}
}

func (l *Logger) write(level, msg string, args ...any) {
	entry := logEntry{
		Time:      time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Component: l.component,
		Msg:       msg,
	}
	if len(args) > 0 {
		entry.Fields = make(map[string]any)
		for i := 0; i+1 < len(args); i += 2 {
			key, ok := args[i].(string)
			if !ok {
				key = fmt.Sprintf("arg%d", i)
			}
			val := args[i+1]
			// error interface has no exported fields so json.Marshal produces {}
			// convert to string so the message is actually visible in logs
			if err, ok := val.(error); ok {
				val = err.Error()
			}
			entry.Fields[key] = val
		}
	}
	b, _ := json.Marshal(entry)
	fmt.Fprintln(l.out, string(b))
}

func (l *Logger) Info(msg string, args ...any)  { l.write("INFO", msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.write("WARN", msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.write("ERROR", msg, args...) }
func (l *Logger) Debug(msg string, args ...any) { l.write("DEBUG", msg, args...) }