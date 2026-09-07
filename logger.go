package main

import (
	"fmt"
	"time"
)

// logf prints a timestamped log line.
func logf(format string, args ...any) {
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
