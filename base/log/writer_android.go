//go:build android

// Android log writer. Lines go through NDK liblog so they show up in logcat.
// stdout from a gomobile library does not.

package log

/*
#cgo LDFLAGS: -llog
#include <android/log.h>
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"
)

const androidLogTag = "Portmaster"

func init() {
	androidLog = writeAndroidLog
}

// logcatFilter, when set, decides which lines reach logcat and how they read.
var logcatFilter func(level Severity, text string) (string, bool)

// SetLogcatFilter sets a filter for the lines written to logcat. Logcat is
// readable by apps with log access and ends up in bug reports, so release
// builds use this to keep user data such as domains and app names out of it.
// Set it before logging starts. The in-memory logs are not affected.
func SetLogcatFilter(filter func(level Severity, text string) (string, bool)) {
	logcatFilter = filter
}

// writeAndroidLog writes a line to logcat with __android_log_write.
func writeAndroidLog(level Severity, text string) {
	if logcatFilter != nil {
		var keep bool
		if text, keep = logcatFilter(level, text); !keep {
			return
		}
	}
	var prio C.int
	switch level {
	case TraceLevel, DebugLevel:
		prio = C.ANDROID_LOG_DEBUG
	case WarningLevel:
		prio = C.ANDROID_LOG_WARN
	case ErrorLevel, CriticalLevel:
		prio = C.ANDROID_LOG_ERROR
	case InfoLevel:
		fallthrough
	default:
		prio = C.ANDROID_LOG_INFO
	}

	ctag := C.CString(androidLogTag)
	ctext := C.CString(text)
	C.__android_log_write(prio, ctag, ctext)
	C.free(unsafe.Pointer(ctag))
	C.free(unsafe.Pointer(ctext))
}
