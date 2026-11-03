//go:build android

package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// partialMaxAge drops a half-finished download nobody resumed, for example
// after the index moved on to new files.
const partialMaxAge = 7 * 24 * time.Hour

var errWrongResumeOffset = errors.New("server resumed the download at another offset")

// partialPath is where the bytes of a download in progress are kept, beside
// the download directory so the existing-file scan never reads them. Writing
// to disk as the bytes arrive is what lets a download continue after the
// process was killed or the network dropped, instead of starting over.
func (d *Downloader) partialPath(url string) string {
	sum := sha256.Sum256([]byte(url))
	dir := filepath.Clean(d.u.cfg.DownloadDirectory) + ".partial"
	return filepath.Join(dir, hex.EncodeToString(sum[:16]))
}

// resumeOffset is the number of bytes of url already on disk from an
// earlier attempt, or 0.
func (d *Downloader) resumeOffset(url string) int64 {
	path := d.partialPath(url)
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if time.Since(info.ModTime()) > partialMaxAge {
		_ = os.Remove(path)
		return 0
	}
	return info.Size()
}

// readBody streams a download into its partial file while counting bytes
// for the UI, then returns the whole file. A 206 reply continues the bytes
// already on disk; anything else starts the file over. On a read error the
// partial file stays, so the next attempt asks only for the rest.
//
// The index is downloaded before d.index is set and is not counted: the UI
// shows artifact bytes only.
func (d *Downloader) readBody(resp *http.Response, url string, offset int64) ([]byte, error) {
	counting := d.index != nil
	if counting {
		d.u.setProgressState(ProgressDownloading)
	}

	path := d.partialPath(url)
	resumed := offset > 0 && resp.StatusCode == http.StatusPartialContent
	if resumed && !contentRangeStartsAt(resp.Header.Get("Content-Range"), offset) {
		_ = os.Remove(path)
		return nil, errWrongResumeOffset
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create partial download directory: %w", err)
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if resumed {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open partial download: %w", err)
	}

	if counting {
		already := int64(0)
		if resumed {
			already = offset
			d.u.addDownloaded(offset)
		}
		if resp.ContentLength > 0 {
			d.u.addTotal(already + resp.ContentLength)
		}
	}
	var body io.Reader = resp.Body
	if counting {
		body = countingReader{r: resp.Body, add: d.u.addDownloaded}
	}
	_, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}

	data, err := os.ReadFile(path)
	_ = os.Remove(path)
	if err != nil {
		return nil, fmt.Errorf("read finished download: %w", err)
	}
	return data, nil
}

// contentRangeStartsAt reports whether a Content-Range header such as
// "bytes 1000-1999/2000" starts at offset.
func contentRangeStartsAt(header string, offset int64) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(header), "bytes ")
	if !ok {
		return false
	}
	start, _, ok := strings.Cut(rest, "-")
	return ok && start == fmt.Sprint(offset)
}
