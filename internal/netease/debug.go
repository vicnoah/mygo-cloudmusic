package netease

import (
	"fmt"
	"os"
)

// debugLog writes stream diagnostics only when explicitly enabled. It is
// used while validating a real account's quality entitlements and is silent
// in normal builds/runs.
func debugLog(format string, args ...any) {
	if os.Getenv("MYGO_DEBUG_NETEASE") == "" {
		return
	}
	line := fmt.Sprintf(format, args...)
	if path := os.Getenv("MYGO_DEBUG_NETEASE_FILE"); path != "" {
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			_, _ = fmt.Fprintln(f, line)
			_ = f.Close()
		}
	}
	_, _ = fmt.Fprintln(os.Stderr, line)
}
