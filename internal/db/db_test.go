package db

import (
	"reflect"
	"strings"
	"testing"
)

func TestAddGetRemove(t *testing.T) {
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r := &Record{Name: "hello", Pkgver: "0.1.0", BuildID: "1", Arch: "x86_64", Manual: true}
	if err := d.Add(r); err != nil {
		t.Fatal(err)
	}
	got, ok := d.Get("hello")
	if !ok || got.FullVersion() != "0.1.0-1" {
		t.Fatalf("Get: %+v %v", got, ok)
	}
	if err := d.Add(r); err == nil {
		t.Fatal("se esperaba error por paquete duplicado")
	}
	if err := d.Remove("hello"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Get("hello"); ok {
		t.Fatal("no deberia existir")
	}
}

func TestPersist(t *testing.T) {
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r := &Record{
		Name: "foo", Pkgver: "1.0", BuildID: "2", Arch: "x86_64",
		Depends: []string{"glibc"},
		Files:   []FileEntry{{Path: "usr/bin/foo", SHA256: strings.Repeat("a", 64)}},
		Dirs:    []string{"usr", "usr/bin"},
		Hooks:   map[string][]byte{"pre-install": []byte("echo hi")},
	}
	if err := d.Add(r); err != nil {
		t.Fatal(err)
	}
	re, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := re.Get("foo")
	if !ok {
		t.Fatal("no se cargaron los registros")
	}
	if !reflect.DeepEqual(got.Files, r.Files) || !reflect.DeepEqual(got.Hooks["pre-install"], r.Hooks["pre-install"]) {
		t.Errorf("roundtrip incompleto: %+v", got)
	}
}

func TestReplace(t *testing.T) {
	root := t.TempDir()
	d, _ := Open(root)
	if err := d.Add(&Record{Name: "p", Pkgver: "1.0.0", BuildID: "1", Manual: true}); err != nil {
		t.Fatal(err)
	}
	if err := d.Replace(&Record{Name: "p", Pkgver: "2.0.0", BuildID: "1"}); err != nil {
		t.Fatal(err)
	}
	got, ok := d.Get("p")
	if !ok || got.Pkgver != "2.0.0" {
		t.Fatalf("Replace no actualizo el registro: %+v", got)
	}
	if err := d.Replace(&Record{Name: "nope", Pkgver: "1"}); err == nil {
		t.Fatal("se esperaba error al reemplazar un paquete no instalado")
	}
	re, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := re.Get("p"); got.Pkgver != "2.0.0" {
		t.Fatalf("no persistio el reemplazo: %+v", got)
	}
}

func TestOwnerMap(t *testing.T) {
	root := t.TempDir()
	d, _ := Open(root)
	d.Add(&Record{Name: "a", Files: []FileEntry{{Path: "usr/bin/a", SHA256: "x"}}, Dirs: []string{"usr"}})
	d.Add(&Record{Name: "b", Symlinks: []SymlinkEntry{{Path: "usr/bin/b", Target: "a"}}})

	owner := d.OwnerMap()
	if owner["usr/bin/a"] != "a" || owner["usr/bin/b"] != "b" || owner["usr"] != "a" {
		t.Errorf("owner map: %v", owner)
	}
}
