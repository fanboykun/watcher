package api

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// tailTraceFile filters retained files before limiting; later polls cannot hide an older run.
func tailTraceFile(ctx context.Context, path, traceID string, n int) ([]string, error) {
	archives, err := filepath.Glob(strings.TrimSuffix(path, ".log") + "-*.log*")
	if err != nil {
		return nil, err
	}
	sort.Strings(archives)
	paths := make([]string, 0, len(archives)+1)
	for _, archive := range archives {
		if strings.HasSuffix(archive, ".log") || strings.HasSuffix(archive, ".log.gz") {
			paths = append(paths, archive)
		}
	}
	paths = append(paths, path)
	lines := make([]string, n)
	count := 0
	foundFile := false
	for _, file := range paths {
		err := scanTraceFile(ctx, file, traceID, func(line string) { lines[count%n] = line; count++ })
		if os.IsNotExist(err) {
			continue
		} // Rotation can remove an archive during a refresh.
		if err != nil {
			return nil, err
		}
		foundFile = true
	}
	if !foundFile {
		return nil, os.ErrNotExist
	}
	if count <= n {
		return lines[:count], nil
	}
	start := count % n
	return append(lines[start:], lines[:start]...), nil
}

func scanTraceFile(ctx context.Context, path, traceID string, appendLine func(string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var reader io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		compressed, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer compressed.Close()
		reader = compressed
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var record map[string]json.RawMessage
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		for _, key := range []string{"poll_id", "request_id", "correlation_id", "network_request_id"} {
			var id string
			if json.Unmarshal(record[key], &id) == nil && id == traceID {
				appendLine(scanner.Text())
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan correlated logs: %w", err)
	}
	return nil
}

// tailFile reads backwards in bounded blocks instead of loading the whole log.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := info.Size()
	const blockSize = 64 * 1024
	const maxWindow = 16 * 1024 * 1024
	chunks := make([][]byte, 0)
	total, newlines := 0, 0
	for offset > 0 && newlines <= n {
		size := int64(blockSize)
		if offset < size {
			size = offset
		}
		offset -= size
		block := make([]byte, size)
		read, readErr := f.ReadAt(block, offset)
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		block = block[:read]
		total += read
		if total > maxWindow {
			return nil, fmt.Errorf("log window exceeds 16 MiB; request fewer lines")
		}
		chunks = append(chunks, block)
		newlines += bytes.Count(block, []byte{'\n'})
	}
	var data bytes.Buffer
	data.Grow(total)
	for i := len(chunks) - 1; i >= 0; i-- {
		data.Write(chunks[i])
	}
	text := data.String()
	if offset > 0 {
		// The first block may start in the middle of a record.
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{}, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines, nil
}
