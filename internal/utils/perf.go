package utils

import (
	"fmt"
	"os"
	"sync/atomic"
)

var verbose int32

func SetVerbose(v bool) {
	if v {
		atomic.StoreInt32(&verbose, 1)
	} else {
		atomic.StoreInt32(&verbose, 0)
	}
}

func IsVerbose() bool {
	return atomic.LoadInt32(&verbose) != 0
}

func Perf(format string, args ...any) {
	if atomic.LoadInt32(&verbose) != 0 {
		fmt.Fprintf(os.Stderr, "[perf] "+format+"\n", args...)
	}
}