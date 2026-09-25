package archive

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
	"yarmouth/internal/metadata"
)

func buildFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "usr", "bin")
	doc := filepath.Join(root, "usr", "share", "doc", "hello")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(doc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(doc, "README"), []byte("documentacion de hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "hello"), []byte("#!/bin/sh\necho hola\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("hello", filepath.Join(bin, "hello-alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "usr", "share", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(t.TempDir(), "hello-0.1.0-1.x86_64.yrm")
}

func TestCreateAndVerify(t *testing.T) {
	dest, out := buildFixture(t)
	opts := CreateOptions{
		Manifest: metadata.Manifest{
			Pkgname: "hello",
			Pkgver:  "0.1.0",
			BuildID: "1",
			Arch:    "x86_64",
			Desc:    "Programa de ejemplo",
			License: "MIT",
			Depends: []string{"glibc"},
		},
		DestDir: dest,
		Hooks:   Hooks{PreInstall: []byte("#!/bin/sh\necho pre\n")},
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Create(f, opts)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 3 {
		t.Errorf("files = %d, want 3 (2 hooks+1 payload): %v", len(res.Files), res.Files)
	}
	p, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if p.Manifest.Pkgname != "hello" || p.Manifest.BuildID != "1" {
		t.Errorf("manifest: %+v", p.Manifest)
	}
	if p.Manifest.DataHash != res.DataHash {
		t.Errorf("datahash inconsistente")
	}
	if err := p.VerifyContent(); err != nil {
		t.Errorf("VerifyContent: %v", err)
	}
	if len(p.Entries) != 10 {
		t.Errorf("entries = %d, want 10", len(p.Entries))
	}
	for _, e := range p.Entries {
		if e.Name == "usr/bin/hello" && e.Typeflag != tar.TypeReg {
			t.Errorf("hello deberia ser archivo regular")
		}
		if e.Name == "usr/bin/hello-alias" && e.Linkname != "hello" {
			t.Errorf("alias deberia apuntar a hello")
		}
		if e.Name == "usr/share/empty/" && e.Typeflag != tar.TypeDir {
			t.Errorf("empty deberia ser directorio")
		}
	}
}

func TestDetectTamper(t *testing.T) {
	dest, out := buildFixture(t)
	opts := CreateOptions{
		Manifest: metadata.Manifest{Pkgname: "hello", Pkgver: "0.1.0", BuildID: "1", Arch: "x86_64"},
		DestDir:  dest,
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(f, opts); err != nil {
		t.Fatal(err)
	}
	f.Close()

	p, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.VerifyContent(); err != nil {
		t.Fatalf("baseline: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	idx := bytes.Index(raw, []byte("echo hola"))
	if idx < 0 {
		t.Fatal("no se encontro el marcador en el payload")
	}
	raw[idx] = 'X'
	var recomp bytes.Buffer
	zw, err := xz.NewWriter(&recomp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, recomp.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	p2, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.VerifyContent(); err == nil {
		t.Fatal("se esperaba deteccion de manipulacion")
	}
}

func TestReproducible(t *testing.T) {
	dest, out1 := buildFixture(t)
	opts := CreateOptions{
		Manifest: metadata.Manifest{Pkgname: "hello", Pkgver: "0.1.0", BuildID: "1", Arch: "x86_64"},
		DestDir:  dest,
	}
	f1, err := os.Create(out1)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := Create(f1, opts)
	f1.Close()
	if err != nil {
		t.Fatal(err)
	}
	b1, err := os.ReadFile(out1)
	if err != nil {
		t.Fatal(err)
	}
	out2 := out1 + ".2"
	f2, err := os.Create(out2)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Create(f2, opts)
	f2.Close()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(out2)
	if err != nil {
		t.Fatal(err)
	}
	if r1.DataHash != r2.DataHash {
		t.Errorf("datahash no reproducible: %s != %s", r1.DataHash, r2.DataHash)
	}
	if string(b1) != string(b2) {
		t.Errorf("paquete no reproducible byte a byte")
	}
}
