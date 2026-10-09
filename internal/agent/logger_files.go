package agent

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// WatcherLogPath uses an immutable ID instead of a user-controlled or renamed name.
func WatcherLogPath(logDir string, watcherID uint) string {
	return filepath.Join(logDir, "watchers", strconv.FormatUint(uint64(watcherID), 10), LogFilename)
}

// scopedLogWriter owns one rotator per ID, shared by polling and manual operations.
// Each slog write is a complete JSON record. Serializing destination writes avoids
// interleaving and competing rotators when watcher goroutines restart.
type scopedLogWriter struct {
	mu         sync.Mutex
	global     io.Writer
	globalFile io.Closer
	logDir     string
	cfg        LogConfig
	files      map[uint]*lumberjack.Logger
	closed     bool
}

func (w *scopedLogWriter) Write(record []byte) (int, error) {
	var identity struct {
		WatcherID uint `json:"watcher_id"`
	}
	if err := json.Unmarshal(record, &identity); err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("log writer closed")
	}
	n, err := w.global.Write(record)
	if err != nil {
		return n, err
	}
	if identity.WatcherID == 0 {
		return n, nil
	}
	file := w.files[identity.WatcherID]
	if file == nil {
		file = &lumberjack.Logger{
			Filename: WatcherLogPath(w.logDir, identity.WatcherID),
			MaxSize:  w.cfg.MaxSizeMB, MaxBackups: w.cfg.MaxBackups,
			MaxAge: w.cfg.MaxAgeDays, Compress: w.cfg.Compress,
		}
		w.files[identity.WatcherID] = file
	}
	return file.Write(record)
}

func (w *scopedLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	err := w.globalFile.Close()
	for _, file := range w.files {
		err = errors.Join(err, file.Close())
	}
	return err
}
