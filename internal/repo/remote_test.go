package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yarmouth/internal/archive"
	"yarmouth/internal/metadata"
)

func buildOne(t *testing.T, repoDir, name, pkgver string) Package {
	t.Helper()
	dest := t.TempDir()
	bin := filepath.Join(dest, "usr", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho "+name+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := metadata.Manifest{Pkgname: name, Pkgver: pkgver, BuildID: "1", Arch: "x86_64"}
	out := filepath.Join(repoDir, name+"-"+pkgver+"-1.x86_64.yrm")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Create(f, archive.CreateOptions{Manifest: m, DestDir: dest}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	return Package{Name: name, Version: pkgver, BuildID: "1", Arch: "x86_64", Size: fi.Size()}
}

func TestLocalRepoFlow(t *testing.T) {
	root := t.TempDir()
	repoDir := t.TempDir()
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := buildOne(t, repoDir, "foobar", "1.0.0")
	p.SHA256 = wholeHash(t, filepath.Join(repoDir, p.Filename()))
	if err := WriteIndexFile(filepath.Join(repoDir, "repodata"), []Package{p}); err != nil {
		t.Fatal(err)
	}

	if err := AddRepo(root, "local", repoDir); err != nil {
		t.Fatal(err)
	}
	if err := AddRepo(root, "local", repoDir); err == nil {
		t.Fatal("se esperaba error por alias duplicado")
	}

	remotes, err := Sync(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 1 || remotes[0].Alias != "local" {
		t.Fatalf("remotes: %+v", remotes)
	}

	m, err := Resolve(root, "foobar")
	if err != nil {
		t.Fatal(err)
	}
	if m.Pkg.Version != "1.0.0" {
		t.Errorf("version: %+v", m.Pkg)
	}

	path, err := FetchPackage(root, m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "foobar-1.0.0-1.x86_64.yrm") {
		t.Errorf("cache path: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if wholeHash(t, path) != p.SHA256 {
		t.Error("sha256 del paquete descargado no coincide")
	}

	if _, err := Resolve(root, "zzz"); err == nil {
		t.Fatal("no deberia encontrar el paquete zzz")
	}
}

func TestResolvePrefersNewest(t *testing.T) {
	root := t.TempDir()
	repoDir := t.TempDir()
	p1 := buildOne(t, repoDir, "bar", "1.0.0")
	p1.SHA256 = wholeHash(t, filepath.Join(repoDir, p1.Filename()))
	p2 := buildOne(t, repoDir, "bar", "2.0.0")
	p2.SHA256 = wholeHash(t, filepath.Join(repoDir, p2.Filename()))
	if err := WriteIndexFile(filepath.Join(repoDir, "repodata"), []Package{p1, p2}); err != nil {
		t.Fatal(err)
	}
	if err := AddRepo(root, "local", repoDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(root); err != nil {
		t.Fatal(err)
	}
	m, err := Resolve(root, "bar")
	if err != nil {
		t.Fatal(err)
	}
	if m.Pkg.Version != "2.0.0" {
		t.Errorf("deberia escoger 2.0.0, escogio: %+v", m.Pkg)
	}
}

func TestRepoRemove(t *testing.T) {
	root := t.TempDir()
	if err := AddRepo(root, "a", "/tmp/a"); err != nil {
		t.Fatal(err)
	}
	if err := AddRepo(root, "b", "/tmp/b"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRepo(root, "a"); err != nil {
		t.Fatal(err)
	}
	remotes, err := ReadRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 1 || remotes[0].Alias != "b" {
		t.Errorf("remotes: %+v", remotes)
	}
}

func wholeHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256sum(data)
	return sum
}
