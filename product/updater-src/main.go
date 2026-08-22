package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const version = "0.5.0"

func verify(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), strings.TrimSpace(want)) {
		return errors.New("package SHA-256 mismatch")
	}
	return nil
}

func safeDestination(root, name string) (string, error) {
	name = filepath.Clean(filepath.FromSlash(name))
	if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", errors.New("unsafe package path")
	}
	dst := filepath.Join(root, name)
	rel, err := filepath.Rel(root, dst)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("package path escapes install root")
	}
	return dst, nil
}

func install(pkg, root string) error {
	z, err := zip.OpenReader(pkg)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if !strings.HasPrefix(filepath.ToSlash(f.Name), "payload/") {
			continue
		}
		name := strings.TrimPrefix(filepath.ToSlash(f.Name), "payload/")
		if name == "" {
			continue
		}
		dst, err := safeDestination(root, name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		tmp := dst + ".update"
		w, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			_ = r.Close()
			return err
		}
		_, err = io.Copy(w, r)
		closeErr := w.Close()
		_ = r.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		_ = os.Remove(dst + ".old")
		if _, statErr := os.Stat(dst); statErr == nil {
			_ = os.Rename(dst, dst+".old")
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	pkg := flag.String("package", "", "verified update ZIP")
	root := flag.String("root", "", "installation root")
	want := flag.String("sha256", "", "expected package SHA-256")
	_ = flag.String("version", "", "target version")
	flag.Parse()
	if *pkg == "" || *root == "" || len(*want) != 64 {
		fmt.Fprintln(os.Stderr, "invalid updater arguments")
		os.Exit(2)
	}
	// The helper exits immediately after spawning us; a short wait avoids Windows file locks.
	time.Sleep(1500 * time.Millisecond)
	if err := verify(*pkg, *want); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	if err := install(*pkg, *root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
}
