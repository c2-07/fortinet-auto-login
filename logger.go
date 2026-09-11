package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

var logOut io.Writer = os.Stdout

// logf prints a timestamped log line.
func logf(format string, args ...any) {
	fmt.Fprintf(logOut, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
