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
// earlier attempt, or 0. A partial file without a saved validator is
// dropped: without If-Range the server could append bytes of a newer
// version of the file.
func (d *Downloader) resumeOffset(url string) int64 {
	path := d.partialPath(url)
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if time.Since(info.ModTime()) > partialMaxAge || d.resumeValidator(url) == "" {
		d.dropPartial(url)
		return 0
	}
	return info.Size()
}

// resumeRequest asks for the rest of a partial download, if there is one,
// and returns the offset it asked from. If-Range makes the server send the
// whole file instead when it changed since the partial file was written.
func (d *Downloader) resumeRequest(req *http.Request, url string) int64 {
	offset := d.resumeOffset(url)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		req.Header.Set("If-Range", d.resumeValidator(url))
	}
	return offset
}

// resumeRefused drops the partial file and reports true when the server
// refused the range, so the caller downloads the file again from the start.
// This happens when the partial file already holds the whole file, for
// example after the process was killed before the finished file was read.
func (d *Downloader) resumeRefused(resp *http.Response, url string, offset int64) bool {
	if offset == 0 || resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return false
	}
	d.dropPartial(url)
	return true
}

func (d *Downloader) validatorPath(url string) string {
	return d.partialPath(url) + ".v"
}

// resumeValidator is the ETag or Last-Modified saved with a partial file.
func (d *Downloader) resumeValidator(url string) string {
	data, err := os.ReadFile(d.validatorPath(url))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// saveValidator keeps what If-Range needs to resume this response later. A
// weak ETag cannot be used with If-Range, so Last-Modified is used then.
func (d *Downloader) saveValidator(url string, resp *http.Response) {
	validator := resp.Header.Get("ETag")
	if validator == "" || strings.HasPrefix(validator, "W/") {
		validator = resp.Header.Get("Last-Modified")
	}
	if validator == "" {
		_ = os.Remove(d.validatorPath(url))
		return
	}
	_ = os.WriteFile(d.validatorPath(url), []byte(validator), 0o600)
}

func (d *Downloader) dropPartial(url string) {
	_ = os.Remove(d.partialPath(url))
	_ = os.Remove(d.validatorPath(url))
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
		d.dropPartial(url)
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
	if !resumed {
		d.saveValidator(url, resp)
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
	d.dropPartial(url)
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
