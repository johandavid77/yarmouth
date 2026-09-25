package metadata

import (
	"bytes"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	src := `
# generado por yarmouth

pkgname = hello
pkgver = 0.1.0
buildid = 1
arch = x86_64
description = Programa de ejemplo
license = MIT
depends = glibc
depends = zlib
file = ` + strings.Repeat("a", 64) + ` file1.txt
file = ` + strings.Repeat("b", 64) + ` file2.txt
datahash = 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`
	m, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.Pkgname != "hello" || m.Pkgver != "0.1.0" || m.BuildID != "1" || m.Arch != "x86_64" {
		t.Errorf("campos basicos incorrectos: %+v", m)
	}
	if len(m.Depends) != 2 || len(m.Files) != 2 {
		t.Errorf("listas: %+v", m)
	}
}

func TestReadErrors(t *testing.T) {
	cases := []string{
		"pkgname=foo\n",
		"kev = valor\n",
		"pkgname = \n",
		"file = solo-un-campo\n",
		"incompleto = 1\n",
	}
	for _, c := range cases {
		if _, err := Read(strings.NewReader(c)); err == nil {
			t.Errorf("se esperaba error para: %q", c)
		}
	}
}

func TestWriteStrict(t *testing.T) {
	m := Manifest{
		Pkgname:  "zfoo",
		Pkgver:   "0.1.0",
		BuildID:  "1",
		Arch:     "x86_64",
		Depends:  []string{"b", "a"},
		Files:    []FileEntry{{SHA256: strings.Repeat("a", 64), Path: "/z"}, {SHA256: strings.Repeat("b", 64), Path: "/a"}},
		DataHash: strings.Repeat("c", 64),
		License:  "MIT",
	}
	var buf bytes.Buffer
	if err := Write(&buf, m); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Depends[0] != "a" || got.Depends[1] != "b" {
		t.Errorf("depends desordenados: %v", got.Depends)
	}
	if got.Files[0].Path != "/a" || got.Files[1].Path != "/z" {
		t.Errorf("files desordenados: %v", got.Files)
	}
	if got.License != "MIT" || got.DataHash != strings.Repeat("c", 64) {
		t.Errorf("roundtrip incompleto: %+v", got)
	}
}

func TestValidate(t *testing.T) {
	bad := Manifest{Pkgname: "x"}
	if err := bad.validate(); err == nil {
		t.Error("se esperaba error por manifest incompleto")
	}
	m := Manifest{Pkgname: "x", Pkgver: "1.0", BuildID: "1", Arch: "x86_64", DataHash: "nope"}
	if err := m.validate(); err == nil {
		t.Error("se esperaba error por datahash corto")
	}
}
