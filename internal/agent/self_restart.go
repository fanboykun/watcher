package agent

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

func powershellEncodedCommand(script string) string {
	words := utf16.Encode([]rune(script))
	data := make([]byte, len(words)*2)
	for i, word := range words {
		binary.LittleEndian.PutUint16(data[i*2:], word)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func powershellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func serviceRestartCommand(nssmPath, serviceName, workDir string) (string, error) {
	for _, value := range []string{nssmPath, serviceName, workDir} {
		if strings.TrimSpace(value) == "" || strings.ContainsRune(value, '\x00') {
			return "", fmt.Errorf("restart requires an NSSM path, service name, and working directory")
		}
	}
	// Delay shutdown until the caller can send its HTTP response. Record errors
	// outside the service process, which cannot report them after it is stopped.
	script := fmt.Sprintf(`Start-Sleep -Seconds 2
& %s restart %s *>> %s
if ($LASTEXITCODE -ne 0) {
    Start-Sleep -Seconds 3
    & %s start %s *>> %s
    exit $LASTEXITCODE
}
`, powershellLiteral(nssmPath), powershellLiteral(serviceName), powershellLiteral(filepath.Join(workDir, "watcher-self-restart.log")),
		powershellLiteral(nssmPath), powershellLiteral(serviceName), powershellLiteral(filepath.Join(workDir, "watcher-self-restart.log")))
	return "powershell.exe -NoProfile -NonInteractive -EncodedCommand " + powershellEncodedCommand(script), nil
}

// ScheduleServiceRestart asks the WMI provider to launch the restart worker.
// The worker belongs to the provider's process tree rather than Watcher's, so
// NSSM's process-tree cleanup cannot kill it while stopping this service.
func ScheduleServiceRestart(nssmPath, serviceName, workDir string) error {
	command, err := serviceRestartCommand(nssmPath, serviceName, workDir)
	if err != nil {
		return err
	}
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$result = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=%s}
if ($result.ReturnValue -ne 0) { throw ('Restart worker creation failed: ' + $result.ReturnValue) }
`, powershellLiteral(command))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", powershellEncodedCommand(script)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("schedule service restart: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}
