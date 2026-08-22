package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func responses(t *testing.T, b []byte) []Response {
	t.Helper()
	var out []Response
	for len(b) > 0 {
		if len(b) < 4 {
			t.Fatal("short frame")
		}
		n := binary.LittleEndian.Uint32(b[:4])
		b = b[4:]
		var r Response
		if err := json.Unmarshal(b[:n], &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
		b = b[n:]
	}
	return out
}

func TestDirectDownloadCompletesOnlyAfterFileFinalized(t *testing.T) {
	payload := []byte("deterministic media")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	d := t.TempDir()
	var wire bytes.Buffer
	nw := &nativeWriter{w: &wire}
	jm := newJobManager()
	runHTTPDownload(nw, jm, Request{DownloadRequest: DownloadRequest{JobID: "one", URL: srv.URL + "/movie.mp4"}}, Settings{OutputDir: d})
	events := responses(t, wire.Bytes())
	if events[len(events)-1].Event != "complete" || events[len(events)-1].Progress != 100 {
		t.Fatalf("last event=%+v", events[len(events)-1])
	}
	b, err := os.ReadFile(filepath.Join(d, "movie.mp4"))
	if err != nil || !bytes.Equal(b, payload) {
		t.Fatalf("file mismatch: %v", err)
	}
	for _, e := range events[:len(events)-1] {
		if e.Progress >= 100 {
			t.Fatalf("premature 100%%: %+v", e)
		}
	}
}

func TestYTDLPPlaylistRouting(t *testing.T) {
	args := ytdlpBaseArgs(Settings{}, ToolsStatus{}, false)
	if !contains(args, "--no-playlist") {
		t.Fatal(args)
	}
	args = ytdlpBaseArgs(Settings{}, ToolsStatus{}, true)
	if !contains(args, "--yes-playlist") {
		t.Fatal(args)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestSanitizedEnvironmentDropsSSLKeyLog(t *testing.T) {
	t.Setenv("SSLKEYLOGFILE", "secret")
	for _, v := range sanitizedChildEnv() {
		if len(v) >= 14 && v[:14] == "SSLKEYLOGFILE=" {
			t.Fatal("secret environment persisted")
		}
	}
}
