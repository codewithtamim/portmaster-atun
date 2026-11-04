//go:build !android

package updates

import (
	"io"
	"net/http"
)

// progressTracker is empty: download progress is only reported on Android.
type progressTracker struct{}

func (u *Updater) beginProgress() {}

func (u *Updater) endProgress(error) {}

// resumeRequest returns 0: desktop always downloads a file from the start.
func (d *Downloader) resumeRequest(*http.Request, string) int64 { return 0 }

func (d *Downloader) resumeRefused(*http.Response, string, int64) bool { return false }

func (d *Downloader) readBody(resp *http.Response, _ string, _ int64) ([]byte, error) {
	return io.ReadAll(resp.Body)
}

func (d *Downloader) countExistingFile(string) {}
