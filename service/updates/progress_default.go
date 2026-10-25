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

func (d *Downloader) readBody(resp *http.Response) ([]byte, error) {
	return io.ReadAll(resp.Body)
}

func (d *Downloader) countExistingFile(string) {}
