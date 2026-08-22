package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultUpdateManifestURL = "https://raw.githubusercontent.com/Tamer723/local-toolbox-updates/main/latest.json"

type UpdateManifest struct {
	Version     string   `json:"version"`
	PublishedAt string   `json:"published_at,omitempty"`
	Notes       []string `json:"notes,omitempty"`
	PackageURL  string   `json:"package_url"`
	SHA256      string   `json:"sha256"`
	Size        int64    `json:"size,omitempty"`
}

type UpdateStatus struct {
	CurrentVersion string   `json:"currentVersion"`
	LatestVersion  string   `json:"latestVersion,omitempty"`
	Available      bool     `json:"available"`
	PublishedAt    string   `json:"publishedAt,omitempty"`
	Notes          []string `json:"notes,omitempty"`
	PackageURL     string   `json:"packageUrl,omitempty"`
	SHA256         string   `json:"sha256,omitempty"`
	Size           int64    `json:"size,omitempty"`
	SourceURL      string   `json:"sourceUrl,omitempty"`
}

func versionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(v), "v"))
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	raw := strings.Split(v, ".")
	out := make([]int, 0, len(raw))
	for _, p := range raw {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	for len(out) < 3 {
		out = append(out, 0)
	}
	return out
}

func compareVersions(a, b string) int {
	av, bv := versionParts(a), versionParts(b)
	n := len(av)
	if len(bv) > n {
		n = len(bv)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(av) {
			ai = av[i]
		}
		if i < len(bv) {
			bi = bv[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func updateHTTPClient() *http.Client {
	return &http.Client{Timeout: 25 * time.Second}
}

func updatePackageHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute}
}

func validateHTTPS(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("مصدر التحديث يجب أن يكون رابط HTTPS صالحًا")
	}
	return nil
}

func fetchUpdateManifest(s Settings) (UpdateManifest, error) {
	src := strings.TrimSpace(s.UpdateManifestURL)
	if src == "" {
		src = defaultUpdateManifestURL
	}
	if err := validateHTTPS(src); err != nil {
		return UpdateManifest{}, err
	}
	req, _ := http.NewRequest(http.MethodGet, src, nil)
	req.Header.Set("User-Agent", "LocalToolbox/"+version)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := updateHTTPClient().Do(req)
	if err != nil {
		return UpdateManifest{}, fmt.Errorf("تعذر الوصول إلى مصدر التحديث: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return UpdateManifest{}, errors.New("مصدر التحديث غير مهيأ بعد على GitHub (404)")
		}
		return UpdateManifest{}, fmt.Errorf("مصدر التحديث أعاد HTTP %d", resp.StatusCode)
	}
	lr := io.LimitReader(resp.Body, 1024*1024)
	var m UpdateManifest
	if err := json.NewDecoder(lr).Decode(&m); err != nil {
		return UpdateManifest{}, errors.New("ملف التحديث غير صالح")
	}
	m.Version = strings.TrimSpace(m.Version)
	m.PackageURL = strings.TrimSpace(m.PackageURL)
	m.SHA256 = strings.ToLower(strings.TrimSpace(m.SHA256))
	if m.Version == "" || m.PackageURL == "" || len(m.SHA256) != 64 {
		return UpdateManifest{}, errors.New("ملف التحديث يفتقد version/package_url/sha256")
	}
	if err := validateHTTPS(m.PackageURL); err != nil {
		return UpdateManifest{}, errors.New("رابط حزمة التحديث غير صالح")
	}
	if _, err := hex.DecodeString(m.SHA256); err != nil {
		return UpdateManifest{}, errors.New("بصمة SHA-256 في ملف التحديث غير صالحة")
	}
	return m, nil
}

func manifestStatus(s Settings, m UpdateManifest) UpdateStatus {
	src := strings.TrimSpace(s.UpdateManifestURL)
	if src == "" {
		src = defaultUpdateManifestURL
	}
	return UpdateStatus{
		CurrentVersion: version,
		LatestVersion:  m.Version,
		Available:      compareVersions(version, m.Version) < 0,
		PublishedAt:    m.PublishedAt,
		Notes:          m.Notes,
		PackageURL:     m.PackageURL,
		SHA256:         m.SHA256,
		Size:           m.Size,
		SourceURL:      src,
	}
}

func handleCheckUpdate(nw *nativeWriter, req Request, s Settings) {
	m, err := fetchUpdateManifest(s)
	if err != nil {
		_ = nw.send(Response{ID: req.ID, Event: "update_error", Message: err.Error(), Version: version, Update: &UpdateStatus{CurrentVersion: version, SourceURL: s.UpdateManifestURL}})
		return
	}
	st := manifestStatus(s, m)
	msg := "أنت تستخدم أحدث إصدار."
	if st.Available {
		msg = "يتوفر تحديث جديد " + st.LatestVersion
	}
	_ = nw.send(Response{ID: req.ID, Event: "update_status", Message: msg, Version: version, Update: &st})
}

func updatesDir() string {
	if app := os.Getenv("LOCALAPPDATA"); app != "" {
		return filepath.Join(app, "LocalToolbox", "updates")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".localtoolbox", "updates")
}

func downloadAndVerifyPackage(nw *nativeWriter, req Request, m UpdateManifest) (string, error) {
	if err := os.MkdirAll(updatesDir(), 0755); err != nil {
		return "", err
	}
	dst := filepath.Join(updatesDir(), "LocalToolbox_update_"+strings.ReplaceAll(m.Version, "/", "_")+".zip")
	tmp := dst + ".part"
	_ = os.Remove(tmp)
	hreq, _ := http.NewRequest(http.MethodGet, m.PackageURL, nil)
	hreq.Header.Set("User-Agent", "LocalToolbox/"+version)
	if m.Size > 200*1024*1024 {
		return "", errors.New("حزمة التحديث أكبر من الحد المسموح 200 MB")
	}
	resp, err := updatePackageHTTPClient().Do(hreq)
	if err != nil {
		return "", fmt.Errorf("تعذر تنزيل حزمة التحديث: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("تعذر تنزيل الحزمة: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 200*1024*1024 {
		return "", errors.New("حزمة التحديث أكبر من الحد المسموح 200 MB")
	}
	if m.Size > 0 && resp.ContentLength > 0 && resp.ContentLength != m.Size {
		return "", errors.New("حجم حزمة التحديث لا يطابق البيانات المنشورة")
	}
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher := sha256.New()
	total := resp.ContentLength
	if total <= 0 {
		total = m.Size
	}
	buf := make([]byte, 256*1024)
	var written int64
	last := time.Time{}
	for {
		n, er := resp.Body.Read(buf)
		if n > 0 {
			if _, e := f.Write(buf[:n]); e != nil {
				return "", e
			}
			_, _ = hasher.Write(buf[:n])
			written += int64(n)
			if time.Since(last) > 350*time.Millisecond {
				last = time.Now()
				pct := 0.0
				if total > 0 {
					pct = float64(written) * 100 / float64(total)
					if pct > 96 {
						pct = 96
					}
				}
				_ = nw.send(Response{ID: req.ID, Event: "update_progress", Message: "جارٍ تنزيل التحديث…", Stage: "تنزيل", Progress: pct, DownloadedBytes: float64(written), TotalBytes: float64(total), Version: version})
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return "", er
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, m.SHA256) {
		_ = os.Remove(tmp)
		return "", errors.New("فشل التحقق من SHA-256؛ تم إلغاء التحديث")
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func handleApplyUpdate(nw *nativeWriter, req Request, s Settings) {
	m, err := fetchUpdateManifest(s)
	if err != nil {
		_ = nw.send(Response{ID: req.ID, Event: "update_error", Message: err.Error(), Version: version})
		return
	}
	if compareVersions(version, m.Version) >= 0 {
		st := manifestStatus(s, m)
		_ = nw.send(Response{ID: req.ID, Event: "update_status", Message: "لا يوجد إصدار أحدث للتثبيت.", Version: version, Update: &st})
		return
	}
	updater := filepath.Join(filepath.Dir(os.Args[0]), "local-toolbox-updater.exe")
	if st, e := os.Stat(updater); e != nil || st.IsDir() {
		_ = nw.send(Response{ID: req.ID, Event: "update_error", Message: "مكوّن التحديث المحلي غير موجود. ثبّت 0.3.0 يدويًا مرة واحدة.", Version: version})
		return
	}
	_ = nw.send(Response{ID: req.ID, Event: "update_progress", Message: "جارٍ تجهيز التحديث…", Stage: "تحقق", Progress: 1, Version: version})
	pkg, err := downloadAndVerifyPackage(nw, req, m)
	if err != nil {
		_ = nw.send(Response{ID: req.ID, Event: "update_error", Message: err.Error(), Version: version})
		return
	}
	_ = nw.send(Response{ID: req.ID, Event: "update_progress", Message: "تم التحقق من الحزمة. جارٍ إعادة تشغيل المكوّن المحلي…", Stage: "تثبيت", Progress: 98, Version: version})
	root := filepath.Dir(os.Args[0])
	cmd := exec.Command(updater, "--package", pkg, "--root", root, "--version", m.Version, "--sha256", m.SHA256)
	cmd.SysProcAttr = hiddenProcessAttributes()
	cmd.Env = sanitizedChildEnv()
	if err := cmd.Start(); err != nil {
		_ = nw.send(Response{ID: req.ID, Event: "update_error", Message: "تعذر تشغيل مكوّن التحديث: " + err.Error(), Version: version})
		return
	}
	_ = nw.send(Response{ID: req.ID, Event: "update_restarting", Message: "تم تنزيل التحديث والتحقق منه. ستتم إعادة تشغيل الإضافة تلقائيًا.", Stage: "إعادة تشغيل", Progress: 99, Version: m.Version, Update: &UpdateStatus{CurrentVersion: version, LatestVersion: m.Version, Available: true}})
	time.Sleep(700 * time.Millisecond)
	os.Exit(0)
}
