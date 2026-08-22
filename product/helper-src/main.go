package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const version = "0.5.0"

type Settings struct {
	OutputDir               string `json:"outputDir"`
	DefaultVideoQuality     string `json:"defaultVideoQuality"`
	DefaultAudioBitrate     int    `json:"defaultAudioBitrate"`
	SubtitleLanguages       string `json:"subtitleLanguages"`
	BrowserSession          string `json:"browserSession"`
	OpenFolderOnComplete    bool   `json:"openFolderOnComplete"`
	ForceIPv4               bool   `json:"forceIPv4"`
	MaxConcurrentDownloads  int    `json:"maxConcurrentDownloads"`
	MaxConcurrentProcessing int    `json:"maxConcurrentProcessing"`
	ConcurrentFragments     int    `json:"concurrentFragments"`
	UpdateManifestURL       string `json:"updateManifestUrl"`
	AutoCheckUpdates        bool   `json:"autoCheckUpdates"`
	AutoInstallUpdates      bool   `json:"autoInstallUpdates"`
	FFmpegPath              string `json:"ffmpegPath,omitempty"`
	FFprobePath             string `json:"ffprobePath,omitempty"`
	YTDLPPath               string `json:"ytdlpPath,omitempty"`
	DenoPath                string `json:"denoPath,omitempty"`
}

type BrowserCookie struct {
	Domain         string  `json:"domain"`
	Path           string  `json:"path"`
	Secure         bool    `json:"secure"`
	HostOnly       bool    `json:"hostOnly"`
	ExpirationDate float64 `json:"expirationDate"`
	Name           string  `json:"name"`
	Value          string  `json:"value"`
}

type ToolInfo struct {
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
}

type ToolsStatus struct {
	FFmpeg  ToolInfo `json:"ffmpeg"`
	FFprobe ToolInfo `json:"ffprobe"`
	YTDLP   ToolInfo `json:"ytdlp"`
	Deno    ToolInfo `json:"deno"`
}

type MediaInfo struct {
	Title     string  `json:"title,omitempty"`
	Uploader  string  `json:"uploader,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	Site      string  `json:"site,omitempty"`
	ID        string  `json:"id,omitempty"`
	URL       string  `json:"url,omitempty"`
}

type nativeWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (nw *nativeWriter) send(resp Response) error {
	nw.mu.Lock()
	defer nw.mu.Unlock()
	if !validEvent(resp.Event) {
		return fmt.Errorf("invalid native event %q", resp.Event)
	}
	resp.ProtocolVersion = ProtocolVersion
	if resp.State == "" {
		resp.State = stateForEvent(resp.Event, resp.Stage)
	}
	if resp.Strategy == "" {
		resp.Strategy = strategyForKind(resp.Kind)
	}
	if resp.State != JobCompleted && resp.Progress >= 100 {
		resp.Progress = 99.5
	}
	if (resp.Event == "error" || resp.Event == "update_error") && resp.Error == nil {
		resp.Error = protocolError(resp.Message)
		if resp.Event == "update_error" {
			resp.Error.Code = ErrorUpdateFailed
		}
		lower := strings.ToLower(resp.Message)
		switch {
		case strings.Contains(lower, "403"):
			resp.Error.Code = ErrorHTTP403
		case strings.Contains(lower, "drm"):
			resp.Error.Code = ErrorDRMProtected
			resp.Error.Retryable = false
		case strings.Contains(lower, "تسجيل دخول") || strings.Contains(lower, "authentication"):
			resp.Error.Code = ErrorAuthenticationRequired
		case strings.Contains(lower, "yt-dlp غير مثبت") || strings.Contains(lower, "ffmpeg مطلوب"):
			resp.Error.Code = ErrorToolMissing
			resp.Error.Retryable = false
		case strings.Contains(lower, "استخراج") || strings.Contains(lower, "extraction"):
			resp.Error.Code = ErrorExtractionFailed
		}
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if err := binary.Write(nw.w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err = nw.w.Write(b)
	return err
}

func readMessage(r io.Reader) (Request, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return Request{}, err
	}
	if n == 0 || n > 64*1024*1024 {
		return Request{}, errors.New("invalid message length")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return Request{}, err
	}
	var req Request
	if err := json.Unmarshal(b, &req); err != nil {
		return Request{}, err
	}
	return req, nil
}

func defaultSettings() Settings {
	home, _ := os.UserHomeDir()
	out := filepath.Join(home, "Downloads", "LocalToolbox")
	if home == "" {
		out = filepath.Join(filepath.Dir(os.Args[0]), "output")
	}
	return Settings{
		OutputDir:               out,
		DefaultVideoQuality:     "1080",
		DefaultAudioBitrate:     192,
		SubtitleLanguages:       "ar,en,tr",
		BrowserSession:          "auto",
		OpenFolderOnComplete:    false,
		ForceIPv4:               true,
		MaxConcurrentDownloads:  2,
		MaxConcurrentProcessing: 1,
		ConcurrentFragments:     4,
		UpdateManifestURL:       defaultUpdateManifestURL,
		AutoCheckUpdates:        true,
		AutoInstallUpdates:      false,
	}
}

func configPath() string {
	if app := os.Getenv("LOCALAPPDATA"); app != "" {
		return filepath.Join(app, "LocalToolbox", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".localtoolbox", "config.json")
}

func loadSettings() Settings {
	s := defaultSettings()
	b, err := os.ReadFile(configPath())
	if err == nil {
		var x Settings
		if json.Unmarshal(b, &x) == nil {
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(b, &raw)
			if x.OutputDir != "" {
				s.OutputDir = x.OutputDir
			}
			if x.DefaultVideoQuality != "" {
				s.DefaultVideoQuality = x.DefaultVideoQuality
			}
			if x.DefaultAudioBitrate != 0 {
				s.DefaultAudioBitrate = x.DefaultAudioBitrate
			}
			if x.SubtitleLanguages != "" {
				s.SubtitleLanguages = x.SubtitleLanguages
			}
			if x.BrowserSession != "" {
				switch x.BrowserSession {
				case "none", "auto":
					s.BrowserSession = x.BrowserSession
				case "chrome", "edge", "brave", "firefox":
					// Migrate legacy direct browser DB access to the safer site-scoped extension bridge.
					s.BrowserSession = "auto"
				}
			}
			s.OpenFolderOnComplete = x.OpenFolderOnComplete
			if _, ok := raw["forceIPv4"]; ok {
				s.ForceIPv4 = x.ForceIPv4
			}
			if x.MaxConcurrentDownloads >= 1 && x.MaxConcurrentDownloads <= 4 {
				s.MaxConcurrentDownloads = x.MaxConcurrentDownloads
			}
			if x.MaxConcurrentProcessing >= 1 && x.MaxConcurrentProcessing <= 2 {
				s.MaxConcurrentProcessing = x.MaxConcurrentProcessing
			}
			if x.ConcurrentFragments == 1 || x.ConcurrentFragments == 2 || x.ConcurrentFragments == 4 || x.ConcurrentFragments == 8 {
				s.ConcurrentFragments = x.ConcurrentFragments
			}
			if strings.TrimSpace(x.UpdateManifestURL) != "" {
				s.UpdateManifestURL = strings.TrimSpace(x.UpdateManifestURL)
			}
			if _, ok := raw["autoCheckUpdates"]; ok {
				s.AutoCheckUpdates = x.AutoCheckUpdates
			}
			if _, ok := raw["autoInstallUpdates"]; ok {
				s.AutoInstallUpdates = x.AutoInstallUpdates
			}
			s.FFmpegPath = x.FFmpegPath
			s.FFprobePath = x.FFprobePath
			s.YTDLPPath = x.YTDLPPath
			s.DenoPath = x.DenoPath
		}
	}
	return s
}

func saveSettings(s Settings) error {
	if s.OutputDir == "" {
		s.OutputDir = defaultSettings().OutputDir
	}
	if s.DefaultVideoQuality == "" {
		s.DefaultVideoQuality = "1080"
	}
	if s.DefaultAudioBitrate != 128 && s.DefaultAudioBitrate != 192 && s.DefaultAudioBitrate != 256 && s.DefaultAudioBitrate != 320 {
		s.DefaultAudioBitrate = 192
	}
	if s.SubtitleLanguages == "" {
		s.SubtitleLanguages = "ar,en,tr"
	}
	if s.BrowserSession != "none" && s.BrowserSession != "auto" {
		s.BrowserSession = "auto"
	}
	if s.MaxConcurrentDownloads < 1 || s.MaxConcurrentDownloads > 4 {
		s.MaxConcurrentDownloads = 2
	}
	if s.MaxConcurrentProcessing < 1 || s.MaxConcurrentProcessing > 2 {
		s.MaxConcurrentProcessing = 1
	}
	if s.ConcurrentFragments != 1 && s.ConcurrentFragments != 2 && s.ConcurrentFragments != 4 && s.ConcurrentFragments != 8 {
		s.ConcurrentFragments = 4
	}
	if strings.TrimSpace(s.UpdateManifestURL) == "" {
		s.UpdateManifestURL = defaultUpdateManifestURL
	}
	if err := os.MkdirAll(filepath.Dir(configPath()), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(configPath(), b, 0644)
}

func validFile(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func findTool(name, custom string) (string, bool) {
	if validFile(custom) {
		return custom, true
	}
	exe, _ := os.Executable()
	base := filepath.Dir(exe)
	filename := name
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(filename), ".exe") {
		filename += ".exe"
	}
	candidates := []string{
		filepath.Join(base, filename),
		filepath.Join(base, "tools", filename),
		filepath.Join(base, "tools", "bin", filename),
		filepath.Join(base, name, "bin", filename),
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		candidates = append(candidates, filepath.Join(la, "Microsoft", "WinGet", "Links", filename))
	}
	if home, _ := os.UserHomeDir(); home != "" {
		candidates = append(candidates, filepath.Join(home, "scoop", "shims", filename))
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		candidates = append(candidates, filepath.Join(pd, "chocolatey", "bin", filename))
	}
	for _, p := range candidates {
		if validFile(p) {
			return p, true
		}
	}
	if p, err := exec.LookPath(filename); err == nil {
		return p, true
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	return "", false
}

func toolVersion(path string, args ...string) string {
	if path == "" {
		return ""
	}
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = hiddenProcessAttributes()
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if len(line) > 120 {
		line = line[:120]
	}
	return strings.TrimSpace(line)
}

type toolCacheState struct {
	mu           sync.Mutex
	status       ToolsStatus
	updatedAt    time.Time
	signature    string
	hasData      bool
	withVersions bool
}

var toolCache toolCacheState

func toolSignature(s Settings) string {
	return strings.Join([]string{s.FFmpegPath, s.FFprobePath, s.YTDLPPath, s.DenoPath}, "|")
}

func discoverTools(s Settings, withVersions bool) ToolsStatus {
	fp, ff := findTool("ffmpeg", s.FFmpegPath)
	pp, pf := findTool("ffprobe", s.FFprobePath)
	yp, yf := findTool("yt-dlp", s.YTDLPPath)
	dp, df := findTool("deno", s.DenoPath)
	t := ToolsStatus{
		FFmpeg:  ToolInfo{Found: ff, Path: fp},
		FFprobe: ToolInfo{Found: pf, Path: pp},
		YTDLP:   ToolInfo{Found: yf, Path: yp},
		Deno:    ToolInfo{Found: df, Path: dp},
	}
	if withVersions {
		if ff {
			t.FFmpeg.Version = toolVersion(fp, "-version")
		}
		if pf {
			t.FFprobe.Version = toolVersion(pp, "-version")
		}
		if yf {
			t.YTDLP.Version = toolVersion(yp, "--version")
		}
		if df {
			t.Deno.Version = toolVersion(dp, "--version")
		}
	}
	return t
}

func getTools(s Settings, force bool, withVersions bool) ToolsStatus {
	sig := toolSignature(s)
	toolCache.mu.Lock()
	defer toolCache.mu.Unlock()

	fresh := toolCache.hasData &&
		toolCache.signature == sig &&
		time.Since(toolCache.updatedAt) < 10*time.Minute

	// A version-aware cache is valid for both UI checks and performance-sensitive jobs.
	if !force && fresh && (!withVersions || toolCache.withVersions) {
		return toolCache.status
	}

	t := discoverTools(s, withVersions)
	toolCache.status = t
	toolCache.updatedAt = time.Now()
	toolCache.signature = sig
	toolCache.hasData = true
	toolCache.withVersions = withVersions
	return t
}

func invalidateToolCache() {
	toolCache.mu.Lock()
	toolCache.hasData = false
	toolCache.withVersions = false
	toolCache.mu.Unlock()
}

func ensureOutputDir(s Settings) string {
	out := s.OutputDir
	if out == "" {
		out = defaultSettings().OutputDir
	}
	_ = os.MkdirAll(out, 0755)
	return out
}

func uniqueMP3Path(input string, s Settings) string {
	dir := ensureOutputDir(s)
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)))
	if base == "" {
		base = "audio"
	}
	p := filepath.Join(dir, base+".mp3")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	return filepath.Join(dir, base+"-"+time.Now().Format("20060102-150405")+".mp3")
}

var invalidWindowsName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

func safeOutputFilename(name string) string {
	name = strings.TrimSpace(name)
	name = invalidWindowsName.ReplaceAllString(name, "_")
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "media"
	}
	if len([]rune(name)) > 180 {
		name = string([]rune(name)[:180])
	}
	return name
}

func uniqueOutputPath(dir, name string) string {
	name = safeOutputFilename(name)
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	stamp := time.Now().Format("20060102-150405")
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, stamp, ext))
}

func guessedNameFromResponse(req Request, resp *http.Response) string {
	if strings.TrimSpace(req.Filename) != "" {
		return safeOutputFilename(req.Filename)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil && params["filename"] != "" {
			return safeOutputFilename(params["filename"])
		}
	}
	baseName := ""
	if u, err := url.Parse(req.URL); err == nil {
		base := strings.TrimSpace(filepath.Base(u.Path))
		if base != "" && base != "." && base != string(filepath.Separator) {
			baseName = safeOutputFilename(base)
		}
	}
	ext := ""
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(ct, "video/mp4"):
		ext = ".mp4"
	case strings.Contains(ct, "video/webm"):
		ext = ".webm"
	case strings.Contains(ct, "audio/mpeg"):
		ext = ".mp3"
	case strings.Contains(ct, "audio/mp4") || strings.Contains(ct, "audio/x-m4a"):
		ext = ".m4a"
	case strings.Contains(ct, "audio/aac"):
		ext = ".aac"
	}
	if baseName != "" {
		if filepath.Ext(baseName) == "" && ext != "" {
			baseName += ext
		}
		return baseName
	}
	return "media-" + time.Now().Format("20060102-150405") + ext
}

func cookiePathMatches(requestPath, cookiePath string) bool {
	if requestPath == "" {
		requestPath = "/"
	}
	if cookiePath == "" {
		cookiePath = "/"
	}
	if requestPath == cookiePath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	return strings.HasSuffix(cookiePath, "/") || (len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/')
}

// cookieHeader applies the browser cookie domain, path, and secure rules again
// at the native boundary. This prevents a malformed or stale extension request
// from attaching unrelated site cookies to a direct media request.
func cookieHeader(cookies []BrowserCookie, target *url.URL) string {
	parts := make([]string, 0, len(cookies))
	host := strings.ToLower(target.Hostname())
	for _, c := range cookies {
		domain := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(c.Domain), "."))
		domainMatches := host == domain
		if !c.HostOnly && domain != "" {
			domainMatches = domainMatches || strings.HasSuffix(host, "."+domain)
		}
		if c.Name == "" || strings.ContainsAny(c.Name+c.Value, "\r\n;") || !domainMatches ||
			(c.Secure && target.Scheme != "https") || !cookiePathMatches(target.EscapedPath(), c.Path) {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func runHTTPDownload(nw *nativeWriter, jm *jobManager, req Request, s Settings) {
	jobID := req.JobID
	if jobID == "" {
		jobID = req.ID
	}
	if jm.isCancelled(jobID) {
		_ = nw.send(Response{Event: "cancelled", JobID: jobID, Kind: "download_detected", Message: "تم إلغاء المهمة.", Version: version})
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: "رابط الوسائط غير صالح.", Version: version})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	jm.setCancelFunc(jobID, cancel)
	defer func() { cancel(); jm.clearCancelFunc(jobID) }()
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: err.Error(), Version: version})
		return
	}
	if req.UserAgent != "" {
		hreq.Header.Set("User-Agent", req.UserAgent)
	}
	if req.Referer != "" {
		hreq.Header.Set("Referer", req.Referer)
	}
	if c := cookieHeader(req.Cookies, u); c != "" {
		hreq.Header.Set("Cookie", c)
	}
	hreq.Header.Set("Accept", "*/*")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true}}
	startedAt := time.Now()
	_ = nw.send(Response{Event: "job_started", JobID: jobID, Kind: "download_detected", Message: "جارٍ بدء التنزيل المباشر…", Stage: "اتصال مباشر", Version: version})
	resp, err := client.Do(hreq)
	if err != nil {
		if jm.isCancelled(jobID) || errors.Is(err, context.Canceled) {
			_ = nw.send(Response{Event: "cancelled", JobID: jobID, Kind: "download_detected", Message: "تم إلغاء المهمة.", Version: version})
			return
		}
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: "تعذر تنزيل الوسائط مباشرة: " + err.Error(), Version: version})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := fmt.Sprintf("الخادم رفض التنزيل المباشر (HTTP %d). جرّب المعالجة عبر yt-dlp.", resp.StatusCode)
		code := ErrorUnavailable
		retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			code, retryable = ErrorAuthenticationRequired, true
		case http.StatusForbidden:
			code, retryable = ErrorHTTP403, true
		case http.StatusNotFound, http.StatusGone:
			code, retryable = ErrorExpiredURL, true
		}
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: message, Stage: "اتصال مباشر", Version: version,
			Error: &ErrorModel{Code: code, Message: message, Retryable: retryable, HTTPStatus: resp.StatusCode}})
		return
	}
	outDir := ensureOutputDir(s)
	finalPath := uniqueOutputPath(outDir, guessedNameFromResponse(req, resp))
	partPath := finalPath + ".part"
	f, err := os.Create(partPath)
	if err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: "تعذر إنشاء ملف الحفظ: " + err.Error(), Version: version})
		return
	}
	completed := false
	defer func() {
		f.Close()
		if !completed {
			_ = os.Remove(partPath)
		}
	}()
	buf := make([]byte, 256*1024)
	total := float64(resp.ContentLength)
	if total < 0 {
		total = 0
	}
	var downloaded int64
	lastEmit, lastBytes := time.Now(), int64(0)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: "تعذر كتابة الملف: " + err.Error(), Version: version})
				return
			}
			downloaded += int64(n)
		}
		now := time.Now()
		if now.Sub(lastEmit) >= 500*time.Millisecond || readErr == io.EOF {
			dt := now.Sub(lastEmit).Seconds()
			speed := 0.0
			if dt > 0 {
				speed = float64(downloaded-lastBytes) / dt
			}
			pct, eta := 0.0, 0.0
			if total > 0 {
				pct = float64(downloaded) / total * 100
				if pct > 99.5 {
					pct = 99.5
				}
				if speed > 0 && total > float64(downloaded) {
					eta = (total - float64(downloaded)) / speed
				}
			}
			_ = nw.send(Response{Event: "progress", JobID: jobID, Kind: "download_detected", Message: "جارٍ التنزيل مباشرة…", Stage: "تنزيل مباشر", Progress: pct, SpeedBytes: speed, DownloadedBytes: float64(downloaded), TotalBytes: total, ETASeconds: eta, ElapsedSeconds: time.Since(startedAt).Seconds(), Version: version})
			lastEmit, lastBytes = now, downloaded
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if jm.isCancelled(jobID) || errors.Is(readErr, context.Canceled) {
				_ = nw.send(Response{Event: "cancelled", JobID: jobID, Kind: "download_detected", Message: "تم إلغاء المهمة.", Version: version})
				return
			}
			_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: "انقطع التنزيل: " + readErr.Error(), Version: version})
			return
		}
	}
	if err := f.Close(); err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: "تعذر إغلاق الملف: " + err.Error(), Version: version})
		return
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "download_detected", Message: "تعذر إنهاء الملف: " + err.Error(), Version: version})
		return
	}
	completed = true
	elapsed := time.Since(startedAt).Seconds()
	_ = nw.send(Response{Event: "complete", JobID: jobID, Kind: "download_detected", Message: "اكتمل التنزيل المباشر.", Stage: "اكتمل", Path: finalPath, Progress: 100, DownloadedBytes: float64(downloaded), TotalBytes: total, ElapsedSeconds: elapsed, Version: version})
	if s.OpenFolderOnComplete {
		_ = openPath(finalPath)
	}
}

func probeDuration(ffprobePath, input string) (float64, error) {
	cmd := exec.Command(ffprobePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", input)
	cmd.SysProcAttr = hiddenProcessAttributes()
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

type laneScheduler struct {
	mu      sync.Mutex
	cond    *sync.Cond
	running int
}

func newLaneScheduler() *laneScheduler {
	l := &laneScheduler{}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *laneScheduler) run(limit int, fn func()) {
	if limit < 1 {
		limit = 1
	}
	go func() {
		l.mu.Lock()
		for l.running >= limit {
			l.cond.Wait()
		}
		l.running++
		l.mu.Unlock()

		defer func() {
			l.mu.Lock()
			l.running--
			l.cond.Broadcast()
			l.mu.Unlock()
		}()
		fn()
	}()
}

type jobManager struct {
	downloads  *laneScheduler
	processing *laneScheduler
	mu         sync.Mutex
	commands   map[string]*exec.Cmd
	cancels    map[string]context.CancelFunc
	cancelled  map[string]bool
}

func newJobManager() *jobManager {
	return &jobManager{
		downloads:  newLaneScheduler(),
		processing: newLaneScheduler(),
		commands:   map[string]*exec.Cmd{},
		cancels:    map[string]context.CancelFunc{},
		cancelled:  map[string]bool{},
	}
}

func (jm *jobManager) enqueueDownload(limit int, fn func()) {
	if limit < 1 || limit > 4 {
		limit = 2
	}
	jm.downloads.run(limit, fn)
}

func (jm *jobManager) enqueueProcessing(limit int, fn func()) {
	if limit < 1 || limit > 2 {
		limit = 1
	}
	jm.processing.run(limit, fn)
}

func (jm *jobManager) setCommand(jobID string, cmd *exec.Cmd) {
	jm.mu.Lock()
	jm.commands[jobID] = cmd
	jm.mu.Unlock()
}
func (jm *jobManager) setCancelFunc(jobID string, cancel context.CancelFunc) {
	jm.mu.Lock()
	jm.cancels[jobID] = cancel
	jm.mu.Unlock()
}
func (jm *jobManager) clearCancelFunc(jobID string) {
	jm.mu.Lock()
	delete(jm.cancels, jobID)
	delete(jm.cancelled, jobID)
	jm.mu.Unlock()
}
func (jm *jobManager) clearCommand(jobID string) {
	jm.mu.Lock()
	delete(jm.commands, jobID)
	delete(jm.cancelled, jobID)
	jm.mu.Unlock()
}
func (jm *jobManager) cancel(jobID string) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	jm.cancelled[jobID] = true
	if cancel := jm.cancels[jobID]; cancel != nil {
		cancel()
	}
	if cmd := jm.commands[jobID]; cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return true
}
func (jm *jobManager) isCancelled(jobID string) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	return jm.cancelled[jobID]
}

func mediaIDFromPath(path string) string {
	base := filepath.Base(path)
	re := regexp.MustCompile(`\[([A-Za-z0-9_-]{6,})\]`)
	m := re.FindStringSubmatch(base)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// resolveExistingOutputPath repairs stale/approximate history paths.
// yt-dlp may end with a different container or a title-normalized filename.
// The media id in [id] is stable, so prefer a real file in the same folder
// with the same id and, when possible, the same extension.
func resolveExistingOutputPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path, true
	}
	dir := filepath.Dir(path)
	id := mediaIDFromPath(path)
	if id == "" {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	wantedExt := strings.ToLower(filepath.Ext(path))
	type candidate struct {
		path     string
		sameExt  bool
		modified time.Time
	}
	var candidates []candidate
	needle := "[" + id + "]"
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(e.Name(), needle) {
			continue
		}
		lower := strings.ToLower(e.Name())
		if strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".ytdl") || strings.HasSuffix(lower, ".temp") {
			continue
		}
		info, _ := e.Info()
		mod := time.Time{}
		if info != nil {
			mod = info.ModTime()
		}
		candidates = append(candidates, candidate{
			path:     filepath.Join(dir, e.Name()),
			sameExt:  wantedExt != "" && strings.EqualFold(filepath.Ext(e.Name()), wantedExt),
			modified: mod,
		})
	}
	if len(candidates) == 0 {
		return "", false
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.sameExt && !best.sameExt {
			best = c
			continue
		}
		if c.sameExt == best.sameExt && c.modified.After(best.modified) {
			best = c
		}
	}
	return best.path, true
}

func nearestExistingDir(path string) string {
	p := path
	if filepath.Ext(p) != "" {
		p = filepath.Dir(p)
	}
	for p != "" {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return ""
}

// openPathResolved opens the exact file when it exists, tries to repair a
// stale yt-dlp filename using its [media-id], and finally falls back to the
// nearest existing containing folder. This makes "open file location" useful
// even when an old history entry points to a renamed/moved output.
func openPathResolved(path string) (opened string, fallback bool, err error) {
	if strings.TrimSpace(path) == "" {
		return "", false, errors.New("empty path")
	}
	if st, statErr := os.Stat(path); statErr == nil {
		if err := openNativePath(path, st.IsDir()); err != nil {
			return "", false, err
		}
		return path, false, nil
	}
	if repaired, ok := resolveExistingOutputPath(path); ok {
		if err := openNativePath(repaired, false); err != nil {
			return "", false, err
		}
		return repaired, true, nil
	}
	if dir := nearestExistingDir(path); dir != "" {
		if err := openNativePath(dir, true); err != nil {
			return "", false, err
		}
		return dir, true, nil
	}
	return "", false, fmt.Errorf("المسار لم يعد موجودًا: %s", path)
}

func openPath(path string) error {
	_, _, err := openPathResolved(path)
	return err
}

func runFFmpegConvert(nw *nativeWriter, jm *jobManager, req Request, s Settings) {
	jobID := req.JobID
	if jobID == "" {
		jobID = req.ID
	}
	if jm.isCancelled(jobID) {
		_ = nw.send(Response{Event: "cancelled", JobID: jobID, Kind: "convert_mp3", Version: version})
		return
	}
	tools := getTools(s, false, false)
	if !tools.FFmpeg.Found || !tools.FFprobe.Found {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "convert_mp3", Message: "FFmpeg/FFprobe غير جاهز. افتح الإعدادات وافحص الأدوات.", Version: version})
		return
	}
	if !validFile(req.Path) {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "convert_mp3", Message: "الملف المحدد غير متاح.", Version: version})
		return
	}
	br := req.Bitrate
	if br != 128 && br != 192 && br != 256 && br != 320 {
		br = s.DefaultAudioBitrate
	}
	if br == 0 {
		br = 192
	}
	duration, _ := probeDuration(tools.FFprobe.Path, req.Path)
	out := uniqueMP3Path(req.Path, s)
	startedAt := time.Now()
	args := []string{"-y", "-i", req.Path, "-vn", "-c:a", "libmp3lame", "-b:a", fmt.Sprintf("%dk", br), "-progress", "pipe:1", "-nostats", out}
	cmd := exec.Command(tools.FFmpeg.Path, args...)
	cmd.SysProcAttr = hiddenProcessAttributes()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "convert_mp3", Message: err.Error(), Version: version})
		return
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "convert_mp3", Message: "تعذر تشغيل FFmpeg: " + err.Error(), Version: version})
		return
	}
	jm.setCommand(jobID, cmd)
	defer jm.clearCommand(jobID)
	_ = nw.send(Response{Event: "job_started", JobID: jobID, Kind: "convert_mp3", Message: "جارٍ التحويل إلى MP3…", Stage: "تحويل الصوت", Version: version, Path: req.Path})
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 4096), 1024*1024)
	last := -1.0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "out_time_us=") || strings.HasPrefix(line, "out_time_ms=") {
			p := strings.SplitN(line, "=", 2)
			if len(p) != 2 {
				continue
			}
			us, _ := strconv.ParseFloat(p[1], 64)
			if duration > 0 {
				pct := (us / 1000000.0) / duration * 100
				if pct < 0 {
					pct = 0
				}
				if pct > 99.5 {
					pct = 99.5
				}
				if pct-last >= 0.5 {
					last = pct
					elapsed := time.Since(startedAt).Seconds()
					mediaSeconds := us / 1000000.0
					rate := 0.0
					eta := 0.0
					if elapsed > 0.2 {
						rate = mediaSeconds / elapsed
						if rate > 0 && duration > mediaSeconds {
							eta = (duration - mediaSeconds) / rate
						}
					}
					_ = nw.send(Response{Event: "progress", JobID: jobID, Kind: "convert_mp3", Message: "جارٍ التحويل إلى MP3…", Stage: "تحويل الصوت", Progress: pct, ElapsedSeconds: elapsed, ETASeconds: eta, ProcessingRate: rate, Version: version})
				}
			}
		}
	}
	err = cmd.Wait()
	if jm.isCancelled(jobID) {
		_ = nw.send(Response{Event: "cancelled", JobID: jobID, Kind: "convert_mp3", Message: "تم إلغاء المهمة.", Version: version})
		return
	}
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if len(msg) > 900 {
			msg = msg[len(msg)-900:]
		}
		if msg == "" {
			msg = err.Error()
		}
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: "convert_mp3", Message: "فشل التحويل: " + msg, Version: version})
		return
	}
	_ = nw.send(Response{Event: "progress", JobID: jobID, Kind: "convert_mp3", Message: "اكتملت المعالجة.", Stage: "اكتمل", Progress: 100, ElapsedSeconds: time.Since(startedAt).Seconds(), Version: version})
	_ = nw.send(Response{Event: "complete", JobID: jobID, Kind: "convert_mp3", Message: "اكتمل تحويل الملف.", Stage: "اكتمل", Path: out, Progress: 100, ElapsedSeconds: time.Since(startedAt).Seconds(), Version: version})
	if s.OpenFolderOnComplete {
		_ = openPath(out)
	}
}

func sanitizedChildEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, e := range env {
		name := e
		if i := strings.IndexByte(e, '='); i >= 0 {
			name = e[:i]
		}
		// Some Windows utilities/security tools leave SSLKEYLOGFILE pointing at an
		// inaccessible virtual volume. Python/urllib3 then fails before HTTPS starts.
		if strings.EqualFold(name, "SSLKEYLOGFILE") {
			continue
		}
		out = append(out, e)
	}
	return out
}

func summarizeYTDLPError(raw string) (string, string) {
	details := strings.TrimSpace(raw)
	lower := strings.ToLower(details)
	if strings.Contains(lower, "virtual_file.log") || strings.Contains(lower, "sslkeylogfile") {
		return "تعذر إنشاء اتصال HTTPS بسبب إعداد SSL قديم في Windows. تم إصلاح هذا السبب في الإصدار 0.2.1؛ أعد المحاولة بعد تحديث الـHelper.", details
	}
	if strings.Contains(lower, "http error 403") || strings.Contains(lower, "403: forbidden") {
		return "رفض الموقع طلب تنزيل الوسائط (403). في YouTube قد يلزم وضع التوافق أو جلسة المتصفح بحسب الفيديو. افتح التفاصيل التقنية إذا استمرت المشكلة.", details
	}
	if strings.Contains(lower, "could not copy chrome cookie database") {
		return "الإصدار القديم حاول قراءة قاعدة Cookies المقفلة في Chrome. الإصدار 0.2.4 يستخدم جسر الجلسة الآمن بدلًا من ذلك؛ أعد تحميل الإضافة والـHelper.", details
	}
	if strings.Contains(lower, "sign in to confirm") || strings.Contains(lower, "login required") {
		return "هذا الرابط يحتاج جلسة موقع. اترك خيار الجلسة على «تلقائي» وأعد المحاولة.", details
	}
	if details == "" {
		return "فشلت العملية دون تفاصيل إضافية.", details
	}
	short := details
	if i := strings.LastIndex(short, "ERROR:"); i >= 0 {
		short = strings.TrimSpace(short[i+len("ERROR:"):])
	}
	if len(short) > 260 {
		short = short[:260] + "…"
	}
	return short, details
}

func normalizeSupportedMediaURL(raw string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", "", errors.New("الرابط غير صالح")
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	path := u.EscapedPath()
	if host == "youtu.be" {
		id := strings.Trim(strings.Split(strings.Trim(path, "/"), "/")[0], " ")
		if id == "" {
			return "", "", errors.New("هذا ليس رابط فيديو YouTube مباشرًا")
		}
		return "https://www.youtube.com/watch?v=" + url.QueryEscape(id), "YouTube", nil
	}
	if host == "youtube.com" || host == "m.youtube.com" {
		id := ""
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		}
		if id == "" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 && (parts[0] == "shorts" || parts[0] == "live") {
				id = parts[1]
			}
		}
		if id == "" {
			return "", "", errors.New("صفحة YouTube الحالية ليست فيديو أو Shorts مباشرًا")
		}
		return "https://www.youtube.com/watch?v=" + url.QueryEscape(id), "YouTube", nil
	}
	if host == "instagram.com" || host == "m.instagram.com" {
		p := strings.ToLower(u.Path)
		if strings.HasPrefix(p, "/reel/") || strings.HasPrefix(p, "/reels/") || strings.HasPrefix(p, "/p/") || strings.HasPrefix(p, "/tv/") {
			return u.String(), "Instagram", nil
		}
		return "", "", errors.New("افتح Reel أو منشور Instagram مباشرًا أولًا")
	}
	if host == "x.com" || host == "twitter.com" || host == "mobile.twitter.com" {
		if strings.Contains(strings.ToLower(u.Path), "/status/") {
			return u.String(), "X", nil
		}
		return "", "", errors.New("افتح منشور X الذي يحتوي الوسائط مباشرة")
	}
	if host == "fb.watch" {
		if strings.Trim(u.Path, "/") != "" {
			return u.String(), "Facebook", nil
		}
		return "", "", errors.New("افتح فيديو Facebook مباشرًا أولًا")
	}
	if host == "facebook.com" || host == "m.facebook.com" || host == "web.facebook.com" {
		p := strings.ToLower(u.Path)
		if strings.Contains(p, "/reel") || strings.Contains(p, "/watch") || strings.Contains(p, "/videos") || strings.Contains(p, "/share/v") || strings.Contains(p, "/share/r") || u.Query().Get("v") != "" {
			return u.String(), "Facebook", nil
		}
		return "", "", errors.New("افتح فيديو أو Reel Facebook مباشرًا أولًا")
	}
	return "", "", errors.New("الموقع غير مدعوم حاليًا. المواقع المدعومة: YouTube وFacebook وInstagram وX")
}

func safeCookieField(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	v = strings.ReplaceAll(v, "	", "")
	return v
}

func makeCookieFile(cookies []BrowserCookie) (string, func(), error) {
	if len(cookies) == 0 {
		return "", func() {}, nil
	}
	f, err := os.CreateTemp("", "localtoolbox-cookies-*.txt")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	if _, err = f.WriteString("# Netscape HTTP Cookie File\n"); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, err
	}
	written := 0
	for _, c := range cookies {
		if c.Name == "" || c.Domain == "" || strings.ContainsAny(c.Name+c.Value, "\r\n	") {
			continue
		}
		domain := c.Domain
		includeSub := "FALSE"
		if !c.HostOnly || strings.HasPrefix(domain, ".") {
			includeSub = "TRUE"
			if !strings.HasPrefix(domain, ".") {
				domain = "." + domain
			}
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		exp := int64(c.ExpirationDate)
		line := fmt.Sprintf("%s	%s	%s	%s	%d	%s	%s\n", safeCookieField(domain), includeSub, safeCookieField(path), secure, exp, safeCookieField(c.Name), safeCookieField(c.Value))
		if _, err = f.WriteString(line); err != nil {
			_ = f.Close()
			cleanup()
			return "", func() {}, err
		}
		written++
	}
	if err = f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if written == 0 {
		cleanup()
		return "", func() {}, nil
	}
	return f.Name(), cleanup, nil
}

func addSessionArgs(args []string, req Request, s Settings) ([]string, func()) {
	cleanup := func() {}
	if strings.TrimSpace(req.UserAgent) != "" {
		args = append(args, "--user-agent", req.UserAgent)
	}
	if s.BrowserSession == "auto" && len(req.Cookies) > 0 {
		if cookiePath, c, err := makeCookieFile(req.Cookies); err == nil && cookiePath != "" {
			args = append(args, "--cookies", cookiePath)
			cleanup = c
		}
	}
	return args, cleanup
}

func ytdlpBaseArgs(s Settings, tools ToolsStatus, playlist bool) []string {
	args := []string{"--windows-filenames", "--newline", "--no-color", "--progress-delta", "0.5"}
	if !playlist {
		args = append(args, "--no-playlist")
	} else {
		args = append(args, "--yes-playlist")
	}
	if s.ForceIPv4 {
		args = append(args, "--force-ipv4")
	}
	fragments := s.ConcurrentFragments
	if fragments != 1 && fragments != 2 && fragments != 4 && fragments != 8 {
		fragments = 4
	}
	if fragments > 1 {
		args = append(args, "--concurrent-fragments", strconv.Itoa(fragments))
	}
	if tools.FFmpeg.Found {
		args = append(args, "--ffmpeg-location", filepath.Dir(tools.FFmpeg.Path))
	}
	if tools.Deno.Found {
		args = append(args, "--js-runtimes", "deno:"+tools.Deno.Path)
	}
	return args
}

var progressRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)%`)
var metricNumberRe = regexp.MustCompile(`-?[0-9]+(?:\.[0-9]+)?`)

func parseMetricFloat(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, "NA") || strings.EqualFold(v, "none") {
		return 0
	}
	m := metricNumberRe.FindString(v)
	if m == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

func ytdlpStageMessage(kind, line string) string {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "[merger]"):
		return "دمج الفيديو والصوت"
	case strings.Contains(l, "[extractaudio]"):
		return "تجهيز ملف MP3"
	case strings.Contains(l, "writing video subtitles to") || strings.Contains(l, "downloading subtitles"):
		return "تنزيل ملفات الترجمة"
	case strings.Contains(l, "writing video thumbnail") || strings.Contains(l, "thumbnail") && kind == "download_thumbnail":
		return "تنزيل الصورة المصغرة"
	case strings.Contains(l, "[download] destination"):
		return "تنزيل الوسائط"
	case strings.Contains(l, "[youtube]") || strings.Contains(l, "[generic]") || strings.Contains(l, "extracting url"):
		return "تحليل الرابط"
	}
	return ""
}

func stageMessage(stage string) string {
	switch stage {
	case "دمج الفيديو والصوت":
		return "جارٍ دمج الفيديو والصوت…"
	case "تجهيز ملف MP3":
		return "جارٍ تجهيز ملف MP3…"
	case "تنزيل ملفات الترجمة":
		return "جارٍ تنزيل ملفات الترجمة…"
	case "تنزيل الصورة المصغرة":
		return "جارٍ تنزيل الصورة المصغرة…"
	case "تنزيل الوسائط":
		return "جارٍ تنزيل الوسائط…"
	case "تحليل الرابط":
		return "جارٍ تحليل الرابط…"
	default:
		return "جارٍ التنفيذ…"
	}
}

func runYTDLPJob(nw *nativeWriter, jm *jobManager, req Request, s Settings, kind string) {
	jobID := req.JobID
	if jobID == "" {
		jobID = req.ID
	}
	if jm.isCancelled(jobID) {
		_ = nw.send(Response{Event: "cancelled", JobID: jobID, Kind: kind, Version: version})
		return
	}
	genericDetected := kind == "download_stream" || kind == "extract_detected_audio" || req.Playlist
	if genericDetected {
		u, err := url.Parse(strings.TrimSpace(req.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: "رابط الوسائط المكتشفة غير صالح.", Version: version})
			return
		}
		req.URL = u.String()
	} else {
		normalizedURL, _, urlErr := normalizeSupportedMediaURL(req.URL)
		if urlErr != nil {
			_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: urlErr.Error(), Version: version})
			return
		}
		req.URL = normalizedURL
	}

	tools := getTools(s, false, false)
	if !tools.YTDLP.Found {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: "yt-dlp غير مثبت. افتح الإعدادات لتثبيته.", Version: version})
		return
	}
	if (kind == "download_video" || kind == "download_audio" || kind == "download_stream" || kind == "extract_detected_audio") && !tools.FFmpeg.Found {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: "FFmpeg مطلوب لهذه العملية.", Version: version})
		return
	}

	outDir := ensureOutputDir(s)
	args := ytdlpBaseArgs(s, tools, req.Playlist)
	args, cleanupSession := addSessionArgs(args, req, s)
	defer cleanupSession()
	if strings.TrimSpace(req.Referer) != "" {
		args = append(args, "--referer", req.Referer)
	}
	args = append(args, "-o", filepath.Join(outDir, "%(title).180B [%(id)s].%(ext)s"))
	args = append(args, "--progress-template",
		"download:__LT_PROGRESS__:%(progress._percent_str)s|%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|%(progress.speed)s|%(progress.eta)s")

	switch kind {
	case "download_video":
		q := req.Quality
		if q == "" {
			q = s.DefaultVideoQuality
		}
		if q == "" {
			q = "1080"
		}
		if q == "best" {
			args = append(args, "-f", "bv*+ba/b")
		} else {
			if _, err := strconv.Atoi(q); err != nil {
				q = "1080"
			}
			args = append(args, "-f", fmt.Sprintf("bv*[height<=%s]+ba/b[height<=%s]", q, q))
		}
		args = append(args, "--merge-output-format", "mp4", "--print", "after_move:__FINAL__:%(filepath)s")
	case "download_audio":
		br := req.Bitrate
		if br == 0 {
			br = s.DefaultAudioBitrate
		}
		if br == 0 {
			br = 192
		}
		args = append(args, "-x", "--audio-format", "mp3", "--audio-quality", fmt.Sprintf("%dK", br), "--print", "after_move:__FINAL__:%(filepath)s")
	case "download_thumbnail":
		args = append(args, "--skip-download", "--write-thumbnail")
	case "download_subtitles":
		langs := strings.TrimSpace(req.Languages)
		if langs == "" {
			langs = s.SubtitleLanguages
		}
		if langs == "" {
			langs = "ar,en,tr"
		}
		args = append(args, "--skip-download", "--write-subs", "--write-auto-subs", "--sub-langs", langs, "--sub-format", "srt/best")
	case "download_stream":
		args = append(args, "-f", "bv*+ba/b", "--merge-output-format", "mp4", "--print", "after_move:__FINAL__:%(filepath)s")
	case "extract_detected_audio":
		br := req.Bitrate
		if br == 0 {
			br = s.DefaultAudioBitrate
		}
		if br == 0 {
			br = 192
		}
		args = append(args, "-x", "--audio-format", "mp3", "--audio-quality", fmt.Sprintf("%dK", br), "--print", "after_move:__FINAL__:%(filepath)s")
	default:
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: "نوع مهمة غير مدعوم.", Version: version})
		return
	}

	args = append(args, req.URL)
	cmd := exec.Command(tools.YTDLP.Path, args...)
	cmd.SysProcAttr = hiddenProcessAttributes()
	cmd.Env = sanitizedChildEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: err.Error(), Version: version})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: err.Error(), Version: version})
		return
	}

	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: "تعذر تشغيل yt-dlp: " + err.Error(), Version: version})
		return
	}
	jm.setCommand(jobID, cmd)
	defer jm.clearCommand(jobID)

	currentStage := "تحليل الرابط"
	_ = nw.send(Response{Event: "job_started", JobID: jobID, Kind: kind, Message: stageMessage(currentStage), Stage: currentStage, Version: version})

	lines := make(chan string, 64)
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 4096), 2*1024*1024)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	go func() { wg.Wait(); close(lines) }()

	finalPath := ""
	var tail []string
	lastProgress := 0.0
	lastEmit := time.Time{}
	var speedBytes, downloadedBytes, totalBytes, etaSeconds float64

	for line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "__FINAL__:") {
			finalPath = strings.TrimSpace(strings.TrimPrefix(t, "__FINAL__:"))
			continue
		}

		if strings.Contains(t, "__LT_PROGRESS__:") {
			payload := t[strings.Index(t, "__LT_PROGRESS__:")+len("__LT_PROGRESS__:"):]
			parts := strings.Split(payload, "|")
			if len(parts) >= 6 {
				if m := progressRe.FindStringSubmatch(parts[0]); len(m) > 1 {
					if raw, e := strconv.ParseFloat(m[1], 64); e == nil {
						pct := raw * 0.95
						if pct > 95 {
							pct = 95
						}
						if pct >= lastProgress {
							lastProgress = pct
						}
					}
				}
				downloadedBytes = parseMetricFloat(parts[1])
				totalBytes = parseMetricFloat(parts[2])
				if totalBytes <= 0 {
					totalBytes = parseMetricFloat(parts[3])
				}
				speedBytes = parseMetricFloat(parts[4])
				etaSeconds = parseMetricFloat(parts[5])
			}
			if currentStage == "تحليل الرابط" {
				currentStage = "تنزيل الوسائط"
			}
			if time.Since(lastEmit) >= 450*time.Millisecond {
				lastEmit = time.Now()
				_ = nw.send(Response{
					Event:           "progress",
					JobID:           jobID,
					Kind:            kind,
					Message:         stageMessage(currentStage),
					Stage:           currentStage,
					Progress:        lastProgress,
					SpeedBytes:      speedBytes,
					DownloadedBytes: downloadedBytes,
					TotalBytes:      totalBytes,
					ETASeconds:      etaSeconds,
					ElapsedSeconds:  time.Since(startedAt).Seconds(),
					Version:         version,
				})
			}
		}

		if stage := ytdlpStageMessage(kind, t); stage != "" && stage != currentStage {
			currentStage = stage
			_ = nw.send(Response{
				Event:           "progress",
				JobID:           jobID,
				Kind:            kind,
				Message:         stageMessage(currentStage),
				Stage:           currentStage,
				Progress:        lastProgress,
				SpeedBytes:      speedBytes,
				DownloadedBytes: downloadedBytes,
				TotalBytes:      totalBytes,
				ETASeconds:      etaSeconds,
				ElapsedSeconds:  time.Since(startedAt).Seconds(),
				Version:         version,
			})
		}

		if t != "" {
			tail = append(tail, t)
			if len(tail) > 12 {
				tail = tail[len(tail)-12:]
			}
		}
	}

	err = cmd.Wait()
	if jm.isCancelled(jobID) {
		_ = nw.send(Response{Event: "cancelled", JobID: jobID, Kind: kind, Message: "تم إلغاء المهمة.", ElapsedSeconds: time.Since(startedAt).Seconds(), Version: version})
		return
	}
	if err != nil {
		msg := strings.Join(tail, "\n")
		if len(msg) > 1200 {
			msg = msg[len(msg)-1200:]
		}
		if msg == "" {
			msg = err.Error()
		}
		short, details := summarizeYTDLPError(msg)
		_ = nw.send(Response{Event: "error", JobID: jobID, Kind: kind, Message: short, Details: details, Stage: currentStage, ElapsedSeconds: time.Since(startedAt).Seconds(), Version: version})
		return
	}

	if finalPath != "" {
		if repaired, ok := resolveExistingOutputPath(finalPath); ok {
			finalPath = repaired
		}
	}
	if finalPath == "" {
		finalPath = outDir
	}
	elapsed := time.Since(startedAt).Seconds()
	_ = nw.send(Response{Event: "progress", JobID: jobID, Kind: kind, Message: "اكتملت المعالجة.", Stage: "اكتمل", Progress: 100, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, ElapsedSeconds: elapsed, Version: version})
	_ = nw.send(Response{Event: "complete", JobID: jobID, Kind: kind, Message: "اكتملت المهمة بنجاح.", Stage: "اكتمل", Path: finalPath, Progress: 100, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, ElapsedSeconds: elapsed, Version: version})
	if s.OpenFolderOnComplete {
		_ = openPath(finalPath)
	}
}

type cachedMediaInfo struct {
	info      MediaInfo
	updatedAt time.Time
}

var mediaInfoCache = struct {
	mu      sync.Mutex
	entries map[string]cachedMediaInfo
}{entries: map[string]cachedMediaInfo{}}

func getCachedMediaInfo(url string) (MediaInfo, bool) {
	mediaInfoCache.mu.Lock()
	defer mediaInfoCache.mu.Unlock()
	c, ok := mediaInfoCache.entries[url]
	if !ok || time.Since(c.updatedAt) > 10*time.Minute {
		if ok {
			delete(mediaInfoCache.entries, url)
		}
		return MediaInfo{}, false
	}
	return c.info, true
}

func putCachedMediaInfo(url string, info MediaInfo) {
	mediaInfoCache.mu.Lock()
	mediaInfoCache.entries[url] = cachedMediaInfo{info: info, updatedAt: time.Now()}
	mediaInfoCache.mu.Unlock()
}

func fetchMediaInfo(nw *nativeWriter, req Request, s Settings) {
	normalizedURL, _, urlErr := normalizeSupportedMediaURL(req.URL)
	if urlErr != nil {
		_ = nw.send(Response{ID: req.ID, Event: "error", Message: urlErr.Error(), Version: version})
		return
	}
	req.URL = normalizedURL
	if info, ok := getCachedMediaInfo(req.URL); ok && !req.Force {
		_ = nw.send(Response{ID: req.ID, Event: "media_info", Message: "تم استخدام معلومات محفوظة مؤقتًا.", Info: &info, Version: version})
		return
	}

	tools := getTools(s, false, false)
	if !tools.YTDLP.Found {
		_ = nw.send(Response{ID: req.ID, Event: "error", Message: "yt-dlp غير مثبت.", Version: version})
		return
	}
	args := ytdlpBaseArgs(s, tools, false)
	args, cleanupSession := addSessionArgs(args, req, s)
	defer cleanupSession()
	args = append(args, "--dump-single-json", "--skip-download", req.URL)
	cmd := exec.Command(tools.YTDLP.Path, args...)
	cmd.SysProcAttr = hiddenProcessAttributes()
	cmd.Env = sanitizedChildEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		short, details := summarizeYTDLPError(string(out))
		_ = nw.send(Response{ID: req.ID, Event: "error", Message: short, Details: details, Version: version})
		return
	}
	var raw struct {
		Title        string  `json:"title"`
		Uploader     string  `json:"uploader"`
		Duration     float64 `json:"duration"`
		Thumbnail    string  `json:"thumbnail"`
		ExtractorKey string  `json:"extractor_key"`
		ID           string  `json:"id"`
		WebpageURL   string  `json:"webpage_url"`
	}
	if json.Unmarshal(out, &raw) != nil {
		_ = nw.send(Response{ID: req.ID, Event: "error", Message: "تعذر تحليل بيانات الفيديو.", Version: version})
		return
	}
	info := MediaInfo{Title: raw.Title, Uploader: raw.Uploader, Duration: raw.Duration, Thumbnail: raw.Thumbnail, Site: raw.ExtractorKey, ID: raw.ID, URL: raw.WebpageURL}
	putCachedMediaInfo(req.URL, info)
	_ = nw.send(Response{ID: req.ID, Event: "media_info", Info: &info, Version: version})
}

func main() {
	nw := &nativeWriter{w: os.Stdout}
	jm := newJobManager()
	for {
		req, err := readMessage(os.Stdin)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return
			}
			return
		}
		s := loadSettings()
		if !validCommand(req.Action) {
			_ = nw.send(Response{ID: req.ID, Event: "error", Message: "أمر غير معروف.", Error: &ErrorModel{Code: "invalid_request", Message: "unknown native command", Retryable: false}, Version: version})
			continue
		}
		if req.ProtocolVersion > ProtocolVersion {
			_ = nw.send(Response{ID: req.ID, Event: "error", Message: "إصدار بروتوكول Native Messaging غير مدعوم.", Error: &ErrorModel{Code: "invalid_request", Message: "unsupported protocol version", Retryable: false}, Version: version})
			continue
		}
		switch req.Action {
		case "ping":
			t := getTools(s, false, false)
			caps := capabilityFlags(t)
			_ = nw.send(Response{ID: req.ID, Event: "pong", Message: "Local Helper متصل", Version: version, Capabilities: &caps})
		case "get_settings":
			_ = nw.send(Response{ID: req.ID, Event: "settings", Settings: &s, Version: version})
		case "save_settings":
			if req.Settings == nil {
				_ = nw.send(Response{ID: req.ID, Event: "error", Message: "إعدادات غير صالحة.", Version: version})
				continue
			}
			ns := *req.Settings
			if err := saveSettings(ns); err != nil {
				_ = nw.send(Response{ID: req.ID, Event: "error", Message: "تعذر حفظ الإعدادات: " + err.Error(), Version: version})
			} else {
				invalidateToolCache()
				ns = loadSettings()
				_ = nw.send(Response{ID: req.ID, Event: "settings_saved", Settings: &ns, Version: version})
			}
		case "check_tools":
			t := getTools(s, req.Force, true)
			_ = nw.send(Response{ID: req.ID, Event: "tools_status", Tools: &t, Version: version})
		case "pick_file":
			p, c, e := pickMediaFile()
			if e != nil {
				_ = nw.send(Response{ID: req.ID, Event: "error", Message: "تعذر فتح نافذة الملفات: " + e.Error(), Version: version})
			} else if c {
				_ = nw.send(Response{ID: req.ID, Event: "file_cancelled", Version: version})
			} else {
				_ = nw.send(Response{ID: req.ID, Event: "file_selected", Path: p, Version: version})
			}
		case "pick_output_folder":
			p, c, e := pickFolder()
			if e != nil {
				_ = nw.send(Response{ID: req.ID, Event: "error", Message: "تعذر اختيار المجلد: " + e.Error(), Version: version})
			} else if c {
				_ = nw.send(Response{ID: req.ID, Event: "folder_cancelled", Version: version})
			} else {
				_ = nw.send(Response{ID: req.ID, Event: "folder_selected", Path: p, Version: version})
			}
		case "open_path":
			p := req.Path
			if p == "" {
				p = ensureOutputDir(s)
			}
			opened, fallback, err := openPathResolved(p)
			if err != nil {
				_ = nw.send(Response{ID: req.ID, Event: "error", Message: "تعذر فتح المسار: " + err.Error(), Version: version})
			} else {
				msg := "تم فتح مكان الملف"
				if fallback {
					if st, e := os.Stat(opened); e == nil && st.IsDir() {
						msg = "الملف لم يعد موجودًا بالاسم المسجل؛ تم فتح المجلد بدلًا منه"
					} else {
						msg = "تم العثور على الملف باسمه الفعلي وفتح مكانه"
					}
				}
				_ = nw.send(Response{ID: req.ID, Event: "path_opened", Path: opened, Message: msg, Version: version})
			}
		case "fetch_info":
			go fetchMediaInfo(nw, req, s)
		case "convert_mp3":
			_ = nw.send(Response{Event: "queued", JobID: req.JobID, Kind: "convert_mp3", Message: "في انتظار مسار المعالجة المحلي.", Stage: "انتظار", Version: version})
			jm.enqueueProcessing(s.MaxConcurrentProcessing, func() { runFFmpegConvert(nw, jm, req, s) })
		case "download_video", "download_audio", "download_thumbnail", "download_subtitles", "download_stream", "extract_detected_audio":
			kind := req.Action
			_ = nw.send(Response{Event: "queued", JobID: req.JobID, Kind: kind, Message: "في انتظار مسار التنزيل.", Stage: "انتظار", Version: version})
			jm.enqueueDownload(s.MaxConcurrentDownloads, func() { runYTDLPJob(nw, jm, req, s, kind) })
		case "download_detected":
			_ = nw.send(Response{Event: "queued", JobID: req.JobID, Kind: "download_detected", Message: "في انتظار التنزيل المباشر.", Stage: "انتظار", Version: version})
			jm.enqueueDownload(s.MaxConcurrentDownloads, func() { runHTTPDownload(nw, jm, req, s) })
		case "cancel_job":
			jm.cancel(req.JobID)
			_ = nw.send(Response{ID: req.ID, Event: "cancel_requested", JobID: req.JobID, Message: "تم طلب إلغاء المهمة.", Version: version})
		case "check_update":
			go handleCheckUpdate(nw, req, s)
		case "apply_update":
			go handleApplyUpdate(nw, req, s)
		}
	}
}
