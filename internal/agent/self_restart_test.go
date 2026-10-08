package agent

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestServiceRestartCommandQuotesPathsAndServiceNames(t *testing.T) {
	command, err := serviceRestartCommand(`C:\Program Files\O'Brien\nssm.exe`, "watcher's service", `C:\apps\watcher`)
	if err != nil {
		t.Fatal(err)
	}
	encoded := strings.TrimPrefix(command, "powershell.exe -NoProfile -NonInteractive -EncodedCommand ")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	words := make([]uint16, len(data)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	script := string(utf16.Decode(words))
	if !strings.Contains(script, `& 'C:\Program Files\O''Brien\nssm.exe' restart 'watcher''s service'`) {
		t.Fatalf("restart arguments were not quoted: %s", script)
	}
	if !strings.HasPrefix(script, "Start-Sleep") || !strings.Contains(script, " start 'watcher''s service'") {
		t.Fatalf("missing response delay or recovery start: %s", script)
	}
}

func TestServiceRestartRejectsIncompleteConfiguration(t *testing.T) {
	for _, args := range [][3]string{{"", "watcher", "dir"}, {"nssm", " ", "dir"}, {"nssm", "watcher", ""}, {"nssm\x00", "watcher", "dir"}} {
		if _, err := serviceRestartCommand(args[0], args[1], args[2]); err == nil {
			t.Fatalf("accepted invalid configuration: %q", args)
		}
	}
}
