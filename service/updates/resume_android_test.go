//go:build android

package updates

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestContentRangeStartsAt(t *testing.T) {
	if !contentRangeStartsAt("bytes 1000-1999/2000", 1000) {
		t.Fatal("matching range refused")
	}
	if contentRangeStartsAt("bytes 0-1999/2000", 1000) {
		t.Fatal("range from the start accepted as a resume")
	}
	if contentRangeStartsAt("", 1000) {
		t.Fatal("missing header accepted")
	}
}

func TestDownloadResumesFromPartialFile(t *testing.T) {
	full := strings.Repeat("0123456789", 100)
	var gotRange, gotIfRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		gotIfRange = r.Header.Get("If-Range")
		if gotRange == "bytes=400-" && gotIfRange == `"v1"` {
			w.Header().Set("Content-Range", "bytes 400-999/1000")
			w.Header().Set("Content-Length", strconv.Itoa(600))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(full[400:]))
			return
		}
		_, _ = w.Write([]byte(full))
	}))
	defer srv.Close()

	dir := t.TempDir()
	d := &Downloader{
		u:          &Updater{cfg: Config{DownloadDirectory: dir + "/download"}},
		httpClient: *srv.Client(),
	}
	url := srv.URL + "/base.dsdl"
	part := d.partialPath(url)
	if err := os.MkdirAll(dir+"/download.partial", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part, []byte(full[:400]), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.validatorPath(url), []byte(`"v1"`), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := d.downloadData(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	if gotRange != "bytes=400-" || gotIfRange != `"v1"` {
		t.Fatalf("Range %q, If-Range %q", gotRange, gotIfRange)
	}
	if string(data) != full {
		t.Fatalf("got %d bytes, want the full file", len(data))
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("partial file left behind after success")
	}
}

// newResumeTest returns a downloader for srv with a partial file of url
// holding partial and, if validator is set, a saved validator.
func newResumeTest(t *testing.T, srv *httptest.Server, url, partial, validator string) *Downloader {
	t.Helper()
	dir := t.TempDir()
	d := &Downloader{
		u:          &Updater{cfg: Config{DownloadDirectory: dir + "/download"}},
		httpClient: *srv.Client(),
	}
	if err := os.MkdirAll(dir+"/download.partial", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.partialPath(url), []byte(partial), 0o600); err != nil {
		t.Fatal(err)
	}
	if validator != "" {
		if err := os.WriteFile(d.validatorPath(url), []byte(validator), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestDownloadRestartsWhenRangeRefused(t *testing.T) {
	full := strings.Repeat("0123456789", 100)
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Range") != "" {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(full))
	}))
	defer srv.Close()

	url := srv.URL + "/index.json"
	d := newResumeTest(t, srv, url, full, `"v1"`)
	data, err := d.downloadData(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != full || requests != 2 {
		t.Fatalf("got %d bytes after %d requests", len(data), requests)
	}
	if _, err := os.Stat(d.validatorPath(url)); !os.IsNotExist(err) {
		t.Fatal("validator left behind after success")
	}
}

func TestDownloadRestartsWhenFileChanged(t *testing.T) {
	newer := strings.Repeat("abcdefghij", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The validator no longer matches, so If-Range sends the whole file.
		w.Header().Set("ETag", `"v2"`)
		_, _ = w.Write([]byte(newer))
	}))
	defer srv.Close()

	url := srv.URL + "/index.json"
	d := newResumeTest(t, srv, url, strings.Repeat("0", 400), `"v1"`)
	data, err := d.downloadData(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != newer {
		t.Fatal("old partial bytes mixed into the newer file")
	}
}

func TestPartialWithoutValidatorStartsOver(t *testing.T) {
	full := strings.Repeat("0123456789", 100)
	var gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		_, _ = w.Write([]byte(full))
	}))
	defer srv.Close()

	url := srv.URL + "/index.json"
	d := newResumeTest(t, srv, url, full[:400], "")
	data, err := d.downloadData(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	if gotRange != "" || string(data) != full {
		t.Fatalf("Range %q, got %d bytes", gotRange, len(data))
	}
}

func TestRefusedRangeIsRetriedOnce(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	}))
	defer srv.Close()

	url := srv.URL + "/index.json"
	d := newResumeTest(t, srv, url, strings.Repeat("0", 400), `"v1"`)
	// The partial file cannot be deleted: the retry must still not resume.
	dir := filepath.Dir(d.partialPath(url))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()

	if _, err := d.downloadData(t.Context(), url); err == nil {
		t.Fatal("download of a refused file succeeded")
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestDownloadOverTheLimitIsDropped(t *testing.T) {
	chunk := make([]byte, 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No Content-Length: the size is only known while reading.
		for range maxIndexDownloadSize>>20 + 2 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := srv.URL + "/index.json"
	d := newResumeTest(t, srv, url, "", "")
	_, err := d.downloadData(t.Context(), url)
	if !errors.Is(err, errDownloadTooLarge) {
		t.Fatalf("got %v, want errDownloadTooLarge", err)
	}
	if _, err := os.Stat(d.partialPath(url)); !os.IsNotExist(err) {
		t.Fatal("partial file of an oversized download left behind")
	}
}

func TestDeclaredSizeOverTheLimitIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 400-999/"+strconv.Itoa(maxIndexDownloadSize+1))
			w.Header().Set("Content-Length", "600")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(make([]byte, 600))
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(maxIndexDownloadSize+1))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	url := srv.URL + "/index.json"
	d := newResumeTest(t, srv, url, strings.Repeat("0", 400), `"v1"`)
	if _, err := d.downloadData(t.Context(), url); !errors.Is(err, errDownloadTooLarge) {
		t.Fatalf("resume: got %v, want errDownloadTooLarge", err)
	}
	if _, err := os.Stat(d.partialPath(url)); !os.IsNotExist(err) {
		t.Fatal("partial file kept after an oversized Content-Range")
	}
	if _, err := d.downloadData(t.Context(), url); !errors.Is(err, errDownloadTooLarge) {
		t.Fatalf("full: got %v, want errDownloadTooLarge", err)
	}
}

func TestPartialAgeIsTakenFromTheValidator(t *testing.T) {
	var gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		_, _ = w.Write([]byte("fresh"))
	}))
	defer srv.Close()

	url := srv.URL + "/index.json"
	d := newResumeTest(t, srv, url, strings.Repeat("0", 400), `"v1"`)
	// Appending keeps the partial file young; the validator shows its start.
	old := time.Now().Add(-partialMaxAge - time.Hour)
	if err := os.Chtimes(d.validatorPath(url), old, old); err != nil {
		t.Fatal(err)
	}
	data, err := d.downloadData(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	if gotRange != "" || string(data) != "fresh" {
		t.Fatalf("Range %q, got %q", gotRange, data)
	}
}

func TestPartialDownloadsArePrunedAndCleared(t *testing.T) {
	dir := t.TempDir()
	u := &Updater{cfg: Config{Name: "Test", DownloadDirectory: dir + "/download"}}
	part := u.partialDir()
	if err := os.MkdirAll(part, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(part, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("fresh")
	write("fresh.v")
	write("stale")
	write("stale.v")
	write("orphan")
	write("lonely.v")
	old := time.Now().Add(-partialMaxAge - time.Hour)
	if err := os.Chtimes(filepath.Join(part, "stale.v"), old, old); err != nil {
		t.Fatal(err)
	}

	u.prunePartialDownloads()
	entries, err := os.ReadDir(part)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "fresh,fresh.v" {
		t.Fatalf("after prune: %v", names)
	}

	u.endProgress(nil)
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("partial downloads left after an applied update")
	}
}
