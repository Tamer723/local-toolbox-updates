package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeDestinationRejectsTraversal(t *testing.T) {
	if _, err := safeDestination(t.TempDir(), "../outside"); err == nil {
		t.Fatal("traversal accepted")
	}
}

func TestInstallPayload(t *testing.T) {
	d := t.TempDir()
	pkg := filepath.Join(d, "p.zip")
	f, _ := os.Create(pkg)
	z := zip.NewWriter(f)
	w, _ := z.Create("payload/extension/manifest.json")
	_, _ = w.Write([]byte(`{"version":"0.5.0"}`))
	_ = z.Close()
	_ = f.Close()
	root := filepath.Join(d, "root")
	if err := install(pkg, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "extension", "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyUpdaterCanInstallV2SideBySide(t *testing.T) {
	d := t.TempDir()
	root := filepath.Join(d, "root")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "local-toolbox-updater.exe")
	if err := os.WriteFile(legacy, []byte("running-0.4.0-updater"), 0755); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(d, "migration.zip")
	f, err := os.Create(pkg)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("payload/local-toolbox-updater-v2.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte("updater-v2")); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = install(pkg, root); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != "running-0.4.0-updater" {
		t.Fatalf("legacy updater was replaced: %q", old)
	}
	got, err := os.ReadFile(filepath.Join(root, "local-toolbox-updater-v2.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "updater-v2" {
		t.Fatalf("v2 updater = %q", got)
	}
}
