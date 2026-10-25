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

// writeAndroidLog writes a line to logcat with __android_log_write.
func writeAndroidLog(level Severity, text string) {
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
