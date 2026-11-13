package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var logOut io.Writer = os.Stdout

const maxLogSize = 2 * 1024 * 1024 // 2 MB size cap before rotation

// logFilePath returns the standard log file location based on platform.
func getLogFilePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "autologin.log")
	}
	return "/tmp/autologin.log"
}

// logf prints a timestamped log line and performs rotation if writing to file.
func logf(format string, args ...any) {
	fmt.Fprintf(logOut, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// rotateLogIfNeeded truncates the log file if it exceeds maxLogSize.
func rotateLogIfNeeded() {
	path := getLogFilePath()
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	if fi.Size() > maxLogSize {
		// Read last 256KB to keep recent context
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 256*1024 {
			trimmed := data[len(data)-256*1024:]
			_ = os.WriteFile(path, trimmed, 0644)
		}
	}
}
