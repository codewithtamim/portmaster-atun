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
	var gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		if gotRange == "bytes=400-" {
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

	data, err := d.downloadData(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	if gotRange != "bytes=400-" {
		t.Fatalf("Range header %q", gotRange)
	}
	if string(data) != full {
		t.Fatalf("got %d bytes, want the full file", len(data))
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("partial file left behind after success")
	}
}
