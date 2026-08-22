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
