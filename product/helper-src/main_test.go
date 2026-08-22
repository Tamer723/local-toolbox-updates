package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestDownloadLaneHonorsConcurrencyLimit(t *testing.T) {
	jm := newJobManager()
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	var running atomic.Int32
	var maximum atomic.Int32
	var done sync.WaitGroup
	done.Add(4)
	for i := 0; i < 4; i++ {
		jm.enqueueDownload(string(rune('a'+i)), 2, func() {
			current := running.Add(1)
			for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
			}
			started <- struct{}{}
			<-release
			running.Add(-1)
			done.Done()
		}, nil)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("download lane did not start available work")
		}
	}
	select {
	case <-started:
		t.Fatal("download lane exceeded its concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	done.Wait()
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrent jobs = %d, want 2", maximum.Load())
	}
}

func TestQueuedCancellationRemovesJobWithoutStartingIt(t *testing.T) {
	jm := newJobManager()
	release := make(chan struct{})
	started := make(chan struct{})
	jm.enqueueProcessing("running", 1, func() { close(started); <-release }, nil)
	<-started

	var ran atomic.Bool
	cancelled := make(chan struct{})
	jm.enqueueProcessing("queued", 1, func() { ran.Store(true) }, func() { close(cancelled) })
	if !jm.cancel("queued") {
		t.Fatal("queued job was not found")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("queued cancellation was not reported promptly")
	}
	close(release)
	time.Sleep(50 * time.Millisecond)
	if ran.Load() {
		t.Fatal("cancelled queued job started")
	}
}

func TestCancellingUnknownJobDoesNotPoisonFutureJobID(t *testing.T) {
	jm := newJobManager()
	if jm.cancel("reused") {
		t.Fatal("unknown cancellation reported success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	jm.setCancelFunc("reused", cancel)
	defer jm.clearCancelFunc("reused")
	if jm.isCancelled("reused") {
		t.Fatal("unknown cancellation leaked into future job")
	}
	select {
	case <-ctx.Done():
		t.Fatal("future job context was cancelled")
	default:
	}
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

func TestDirectDownloadScopesCookiesToRequest(t *testing.T) {
	u := "https://media.example.com/video/item.mp4"
	host := "media.example.com"
	cookies := []BrowserCookie{
		{Domain: host, Path: "/video", Secure: true, HostOnly: true, Name: "allowed", Value: "yes"},
		{Domain: host, Path: "/account", Secure: true, HostOnly: true, Name: "wrong_path", Value: "no"},
		{Domain: "unrelated.test", Path: "/", Secure: true, Name: "wrong_domain", Value: "no"},
		{Domain: host, Path: "/", Secure: true, HostOnly: true, Name: "bad", Value: "line\nbreak"},
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	if got := cookieHeader(cookies, parsed); got != "allowed=yes" {
		t.Fatalf("scoped cookie header = %q", got)
	}

	// runHTTPDownload uses its own client, so use its plain-HTTP equivalent to
	// verify that Secure cookies are not sent over an insecure transport.
	plain, _ := url.Parse("http://" + parsed.Host + parsed.Path)
	if got := cookieHeader(cookies, plain); got != "" {
		t.Fatalf("secure cookie leaked over HTTP: %q", got)
	}
}

func TestDirectDownloadReportsStructuredHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()
	var wire bytes.Buffer
	runHTTPDownload(&nativeWriter{w: &wire}, newJobManager(), Request{DownloadRequest: DownloadRequest{JobID: "denied", URL: srv.URL}}, Settings{OutputDir: t.TempDir()})
	events := responses(t, wire.Bytes())
	last := events[len(events)-1]
	if last.Event != "error" || last.Error == nil || last.Error.Code != ErrorHTTP403 || last.Error.HTTPStatus != http.StatusForbidden || !last.Error.Retryable {
		t.Fatalf("unexpected 403 response: %+v", last)
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
