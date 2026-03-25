package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	exePath, err := os.Executable()
	if err != nil {
		showError("Watcher Installer", fmt.Sprintf("Could not determine installer location.\r\n\r\n%s", err))
		os.Exit(1)
	}

	baseDir := filepath.Dir(exePath)
	scriptPath := filepath.Join(baseDir, "shell", "install-watcher.ps1")
	if _, err := os.Stat(scriptPath); err != nil {
		showError("Watcher Installer", fmt.Sprintf("Installer script not found.\r\n\r\nExpected:\r\n%s", scriptPath))
		os.Exit(1)
	}

	debugMode := false
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, "--debug") || strings.EqualFold(arg, "-debug") {
			debugMode = true
			break
		}
	}

	args := []string{
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-STA",
		"-File", scriptPath,
		"-GuiHost",
	}
	if debugMode {
		args = append(args, "-DebugMode")
	}

	cmd := exec.Command("powershell.exe", args...)
	if !debugMode {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	if err := cmd.Start(); err != nil {
		showError("Watcher Installer", fmt.Sprintf("Failed to launch installer.\r\n\r\n%s", err))
		os.Exit(1)
	}
}

func showError(title, message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")
	_, _, _ = messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(message))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))),
		uintptr(0x00000010),
	)
}
