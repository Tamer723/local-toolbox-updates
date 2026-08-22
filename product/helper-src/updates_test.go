package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdaterPathPrefersV2AndFallsBackToLegacy(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "local-toolbox-updater.exe")
	v2 := filepath.Join(root, "local-toolbox-updater-v2.exe")
	if err := os.WriteFile(legacy, []byte("legacy"), 0755); err != nil {
		t.Fatal(err)
	}
	if got, ok := updaterPath(root); !ok || got != legacy {
		t.Fatalf("legacy fallback = %q, %v", got, ok)
	}
	if err := os.WriteFile(v2, []byte("v2"), 0755); err != nil {
		t.Fatal(err)
	}
	if got, ok := updaterPath(root); !ok || got != v2 {
		t.Fatalf("v2 preference = %q, %v", got, ok)
	}
}
