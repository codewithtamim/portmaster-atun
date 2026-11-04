//go:build android

package updates

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
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
