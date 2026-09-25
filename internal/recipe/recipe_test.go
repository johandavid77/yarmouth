package recipe

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yarmouth/internal/archive"
)

func writeRecipe(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pkg.yarmouth")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mkSource(t *testing.T, name, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func shaOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func bodyFor(t *testing.T, src string, extra string) string {
	t.Helper()
	return "pkgname = saludo\npkgver = 1.0\nbuildid = 1\narch = x86_64\nlicense = MIT\n" +
		"sources = " + src + "\nsha256 = " + shaOf(t, src) + "\n" + extra
}

func TestReadRecipe(t *testing.T) {
	src := mkSource(t, "hello.sh", "#!/bin/sh\necho hola\n", 0o755)
	path := writeRecipe(t, bodyFor(t, src,
		"prepare = true\nbuild = chmod +x $SRCDIR/hello.sh\ninstall = install -Dm755 $SRCDIR/hello.sh $DESTDIR/usr/bin/saludo\n"+
			"post-install = echo instalado\n"))
	rec, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Manifest.Pkgname != "saludo" || rec.Manifest.Pkgver != "1.0" {
		t.Fatalf("manifest incorrecto: %+v", rec.Manifest)
	}
	if len(rec.Sources) != 1 || len(rec.SHA256) != 1 {
		t.Fatalf("sources/sha256: %v %v", rec.Sources, rec.SHA256)
	}
	if !strings.Contains(rec.Build, "chmod +x") || !strings.Contains(rec.Install, "install -D") {
		t.Fatalf("build/install: %q %q", rec.Build, rec.Install)
	}
	if string(rec.Hooks.PostInstall) != "echo instalado" {
		t.Fatalf("hook post-install: %q", rec.Hooks.PostInstall)
	}
}

func TestBuildFromLocalSource(t *testing.T) {
	src := mkSource(t, "hello.sh", "#!/bin/sh\necho hola\n", 0o755)
	path := writeRecipe(t, bodyFor(t, src,
		"build = chmod +x $SRCDIR/hello.sh\ninstall = install -Dm755 $SRCDIR/hello.sh $DESTDIR/usr/bin/saludo\n"))
	out := t.TempDir()
	artifact, err := Build(path, out)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := archive.Open(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Pkgname != "saludo" || pkg.Manifest.Arch != "x86_64" {
		t.Fatalf("manifest: %+v", pkg.Manifest)
	}
	found := false
	for _, e := range pkg.Entries {
		if e.Name == "usr/bin/saludo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no se empaqueto usr/bin/saludo: %v", pkg.Entries)
	}
	if err := pkg.VerifyContent(); err != nil {
		t.Fatalf("contenido invalido: %v", err)
	}
}

func TestBuildFromTarballUsesSrcdir(t *testing.T) {
	tarball := filepath.Join(t.TempDir(), "tool-1.0.tar.gz")
	f, err := os.Create(tarball)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	script := "#!/bin/sh\ncat \"$SRCDIR/msg.txt\"\n"
	entries := []struct {
		name string
		data string
		mode int64
	}{
		{"tool-1.0/", "", 0o755},
		{"tool-1.0/msg.txt", "desde el tarball\n", 0o644},
		{"tool-1.0/tool.sh", script, 0o755},
	}
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.data)), Typeflag: tar.TypeReg}
		if strings.HasSuffix(e.name, "/") {
			hdr.Typeflag = tar.TypeDir
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	path := writeRecipe(t, bodyFor(t, tarball,
		"install = install -Dm755 $SRCDIR/tool.sh $DESTDIR/usr/bin/tool\n"))
	out := t.TempDir()
	artifact, err := Build(path, out)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := archive.Open(artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range pkg.Entries {
		if e.Name == "usr/bin/tool" {
			return
		}
	}
	t.Fatalf("no se empaqueto usr/bin/tool: %v", pkg.Entries)
}

func TestBuildSHA256Mismatch(t *testing.T) {
	src := mkSource(t, "hello.sh", "#!/bin/sh\necho hola\n", 0o755)
	path := writeRecipe(t,
		"pkgname = saludo\npkgver = 1.0\nbuildid = 1\narch = x86_64\nlicense = MIT\n"+
			"sources = "+src+"\nsha256 = "+strings.Repeat("0", 64)+"\n"+
			"install = true\n")
	_, err := Build(path, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no coincide") {
		t.Fatalf("se esperaba error de sha256, got: %v", err)
	}
}

func TestBuildStepFailure(t *testing.T) {
	src := mkSource(t, "hello.sh", "#!/bin/sh\necho hola\n", 0o755)
	path := writeRecipe(t, bodyFor(t, src, "install = false\n"))
	_, err := Build(path, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "paso \"install\" fallo") {
		t.Fatalf("se esperaba error del paso install, got: %v", err)
	}
}

func TestBuildRejectsMissingSHA256(t *testing.T) {
	src := mkSource(t, "hello.sh", "#!/bin/sh\n", 0o755)
	path := writeRecipe(t,
		"pkgname = saludo\npkgver = 1.0\nbuildid = 1\narch = x86_64\nlicense = MIT\n"+
			"sources = "+src+"\ninstall = true\n")
	if _, err := ReadFile(path); err == nil {
		t.Fatal("se esperaba error: falta sha256")
	}
}
