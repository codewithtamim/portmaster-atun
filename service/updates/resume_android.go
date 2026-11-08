//go:build android

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/safing/portmaster/base/log"
)

// partialMaxAge drops a half-finished download nobody resumed, for example
// after the index moved on to new files. The age is taken from the validator
// file, which is written once when a download starts: appending to the
// partial file refreshes its own ModTime.
const partialMaxAge = 7 * 24 * time.Hour

// maxIndexDownloadSize caps an index download. Indexes are small JSON files.
const maxIndexDownloadSize = 32 << 20

var (
	errWrongResumeOffset = errors.New("server resumed the download at another offset")
	errDownloadTooLarge  = errors.New("download is larger than allowed")
)

// downloadLimit is the most bytes one download may write to disk. The index
// does not list artifact sizes, so artifacts are capped like unpacking.
func (d *Downloader) downloadLimit() int64 {
	if d.index == nil {
		return maxIndexDownloadSize
	}
	return MaxUnpackSize
}

func (d *Downloader) partialDir() string {
	return d.u.partialDir()
}

func (u *Updater) partialDir() string {
	return filepath.Clean(u.cfg.DownloadDirectory) + ".partial"
}

// partialPath is where the bytes of a download in progress are kept, beside
// the download directory so the existing-file scan never reads them. Writing
// to disk as the bytes arrive is what lets a download continue after the
// process was killed or the network dropped, instead of starting over.
func (d *Downloader) partialPath(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(d.partialDir(), hex.EncodeToString(sum[:16]))
}

// resumeOffset is the number of bytes of url already on disk from an
// earlier attempt, or 0. A partial file without a saved validator is
// dropped: without If-Range the server could append bytes of a newer
// version of the file.
func (d *Downloader) resumeOffset(url string) int64 {
	info, err := os.Stat(d.partialPath(url))
	if err != nil {
		return 0
	}
	started, err := os.Stat(d.validatorPath(url))
	if err != nil || time.Since(started.ModTime()) > partialMaxAge ||
		d.resumeValidator(url) == "" || info.Size() >= d.downloadLimit() {
		d.dropPartial(url)
		return 0
	}
	return info.Size()
}

// resumeRequest asks for the rest of a partial download, if there is one,
// and returns the offset it asked from. If-Range makes the server send the
// whole file instead when it changed since the partial file was written.
func (d *Downloader) resumeRequest(req *http.Request, url string) int64 {
	if fromStart(req.Context()) {
		d.dropPartial(url)
		return 0
	}
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
//
// The retry asks without a range, so it is refused at most once.
func (d *Downloader) resumeRefused(resp *http.Response, url string, offset int64) bool {
	if offset == 0 || resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return false
	}
	d.dropPartial(url)
	return true
}

type fromStartKey struct{}

// downloadFromStart marks the retry after a refused range: it must not
// resume, even if the partial file could not be deleted.
func downloadFromStart(ctx context.Context) context.Context {
	return context.WithValue(ctx, fromStartKey{}, true)
}

func fromStart(ctx context.Context) bool {
	v, _ := ctx.Value(fromStartKey{}).(bool)
	return v
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
// partial file stays, so the next attempt asks only for the rest. On a write
// error, for example a full disk, or a download over the size limit, it is
// deleted.
//
// The index is downloaded before d.index is set and is not counted: the UI
// shows artifact bytes only.
func (d *Downloader) readBody(resp *http.Response, url string, offset int64) ([]byte, error) {
	counting := d.index != nil
	if counting {
		d.u.setProgressState(ProgressDownloading)
	}

	path := d.partialPath(url)
	limit := d.downloadLimit()
	resumed := offset > 0 && resp.StatusCode == http.StatusPartialContent
	if resumed {
		contentRange := resp.Header.Get("Content-Range")
		if !contentRangeStartsAt(contentRange, offset) {
			d.dropPartial(url)
			return nil, errWrongResumeOffset
		}
		if total, ok := contentRangeTotal(contentRange); ok && total > limit {
			d.dropPartial(url)
			return nil, fmt.Errorf("%w: %d bytes", errDownloadTooLarge, total)
		}
	} else if resp.ContentLength > limit {
		d.dropPartial(url)
		return nil, fmt.Errorf("%w: %d bytes", errDownloadTooLarge, resp.ContentLength)
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

	already := int64(0)
	if resumed {
		already = offset
	}
	if counting {
		if resumed {
			d.u.addDownloaded(offset)
		}
		if resp.ContentLength > 0 {
			d.u.addTotal(already + resp.ContentLength)
		}
	}
	// One byte over the limit is read to tell a full-size file from a
	// larger one.
	var body io.Reader = io.LimitReader(resp.Body, limit-already+1)
	if counting {
		body = countingReader{r: body, add: d.u.addDownloaded}
	}
	out := &writeErrorRecorder{w: file}
	written, copyErr := io.Copy(out, body)
	closeErr := file.Close()
	switch {
	case out.err != nil:
		d.dropPartial(url)
		return nil, fmt.Errorf("write partial download: %w", out.err)
	case closeErr != nil:
		d.dropPartial(url)
		return nil, closeErr
	case already+written > limit:
		d.dropPartial(url)
		return nil, fmt.Errorf("%w: over %d bytes", errDownloadTooLarge, limit)
	case copyErr != nil:
		return nil, copyErr
	}

	data, err := os.ReadFile(path)
	d.dropPartial(url)
	if err != nil {
		return nil, fmt.Errorf("read finished download: %w", err)
	}
	return data, nil
}

// writeErrorRecorder keeps the error of the file side of a copy, to tell it
// from a network read error.
type writeErrorRecorder struct {
	w   io.Writer
	err error
}

func (r *writeErrorRecorder) Write(p []byte) (int, error) {
	n, err := r.w.Write(p)
	if err != nil {
		r.err = err
	}
	return n, err
}

// contentRangeTotal returns the total size of a Content-Range header such as
// "bytes 1000-1999/2000", if the server sent one.
func contentRangeTotal(header string) (int64, bool) {
	_, total, ok := strings.Cut(strings.TrimSpace(header), "/")
	if !ok || total == "*" {
		return 0, false
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// clearPartialDownloads deletes all partial downloads. Called after an update
// was applied, when nothing is left to resume.
func (u *Updater) clearPartialDownloads() {
	if err := os.RemoveAll(u.partialDir()); err != nil {
		log.Debugf("updates/%s: failed to clear partial downloads: %s", u.cfg.Name, err)
	}
}

// prunePartialDownloads deletes partial downloads that are too old to resume
// and files left without their pair. Called when an attempt begins.
func (u *Updater) prunePartialDownloads() {
	dir := u.partialDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".v") {
			if _, err := os.Stat(filepath.Join(dir, strings.TrimSuffix(name, ".v"))); err != nil {
				_ = os.Remove(filepath.Join(dir, name))
			}
			continue
		}
		started, err := os.Stat(filepath.Join(dir, name+".v"))
		if err != nil || time.Since(started.ModTime()) > partialMaxAge {
			_ = os.Remove(filepath.Join(dir, name))
			_ = os.Remove(filepath.Join(dir, name+".v"))
		}
	}
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
