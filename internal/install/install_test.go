package install

import (
	"os"
	"path/filepath"
	"testing"

	"yarmouth/internal/archive"
	"yarmouth/internal/db"
	"yarmouth/internal/metadata"
)

func buildPackage(t *testing.T, name string, deps []string, hooks archive.Hooks, extra map[string]string) string {
	return buildPackageV(t, name, "1.0.0", deps, hooks, extra)
}

func buildPackageV(t *testing.T, name, version string, deps []string, hooks archive.Hooks, extra map[string]string) string {
	t.Helper()
	dest := t.TempDir()
	bin := filepath.Join(dest, "usr", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho "+name+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dest, "var", "share", name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(name, filepath.Join(bin, name+"-alias")); err != nil {
		t.Fatal(err)
	}
	for rel, content := range extra {
		p := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := metadata.Manifest{
		Pkgname: name, Pkgver: version, BuildID: "1", Arch: "x86_64",
		Desc: "paquete de pruebas", Depends: deps,
	}
	out := filepath.Join(t.TempDir(), name+"-"+version+"-1.x86_64.yrm")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Create(f, archive.CreateOptions{Manifest: m, DestDir: dest, Hooks: hooks}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return out
}

func openPkg(t *testing.T, path string) *archive.Package {
	t.Helper()
	p, err := archive.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstallRemove(t *testing.T) {
	root := t.TempDir()
	hooks := archive.Hooks{
		PreInstall: []byte("echo install > .pre-installed"),
		PostRemove: []byte("echo remove > .post-removed"),
	}
	pkgPath := buildPackage(t, "hello", nil, hooks, nil)
	d, err := db.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(openPkg(t, pkgPath), d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(root, "usr/bin/hello"),
		filepath.Join(root, "usr/bin/hello-alias"),
		filepath.Join(root, "var/share/hello"),
		filepath.Join(root, ".pre-installed"),
	} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("falta %s: %v", p, err)
		}
	}
	if _, ok := d.Get("hello"); !ok {
		t.Fatal("no se registro el paquete en la db")
	}
	rec, _ := d.Get("hello")
	if rec.SHA256 == "" || rec.FullVersion() != "1.0.0-1" {
		t.Errorf("rec: %+v", rec)
	}

	if err := Install(openPkg(t, pkgPath), d, root, Options{}); err == nil {
		t.Fatal("se esperaba error de paquete ya instalado")
	}

	if err := RemovePackage("hello", d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "usr/bin/hello")); !os.IsNotExist(err) {
		t.Errorf("el binario deberia estar eliminado: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".post-removed")); err != nil {
		t.Errorf("faltaria el hook post-remove: %v", err)
	}
	if _, ok := d.Get("hello"); ok {
		t.Fatal("el paquete deberia estar fuera de la db")
	}
}

func TestConflictUntracked(t *testing.T) {
	root := t.TempDir()
	pkgPath := buildPackage(t, "hello", nil, archive.Hooks{}, nil)
	if err := os.MkdirAll(filepath.Join(root, "usr/bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "usr/bin/hello"), []byte("otra cosa"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, _ := db.Open(root)
	if err := Install(openPkg(t, pkgPath), d, root, Options{}); err == nil {
		t.Fatal("se esperaba conflicto con archivo no registrado")
	}
	if err := Install(openPkg(t, pkgPath), d, root, Options{Force: true}); err != nil {
		t.Fatalf("con -f deberia instalar: %v", err)
	}
}

func TestConflictOwnedByOther(t *testing.T) {
	root := t.TempDir()
	pa := buildPackage(t, "aa", nil, archive.Hooks{}, nil)
	pb := buildPackage(t, "bb", nil, archive.Hooks{}, map[string]string{"usr/bin/aa": "contenido de bb"})
	d, _ := db.Open(root)
	if err := Install(openPkg(t, pa), d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := Install(openPkg(t, pb), d, root, Options{}); err == nil {
		t.Fatal("se esperaba conflicto: bb quiere escribir usr/bin/aa de aa")
	}
	if err := Install(openPkg(t, pb), d, root, Options{Force: true}); err != nil {
		t.Fatalf("con -f deberia permitirse: %v", err)
	}
	if _, ok := d.Get("bb"); !ok {
		t.Fatal("bb deberia quedar instalado")
	}
}

func TestDependentsBlockRemove(t *testing.T) {
	root := t.TempDir()
	pa := buildPackage(t, "aaa", nil, archive.Hooks{}, nil)
	pb := buildPackage(t, "bbb", []string{"aaa"}, archive.Hooks{}, nil)
	d, _ := db.Open(root)
	if err := Install(openPkg(t, pa), d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := Install(openPkg(t, pb), d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := RemovePackage("aaa", d, root, Options{}); err == nil {
		t.Fatal("se esperaba que bbb bloquease la baja de aaa")
	}
	if err := RemovePackage("aaa", d, root, Options{Force: true}); err != nil {
		t.Fatalf("con -f deberia permitirse: %v", err)
	}
	if _, ok := d.Get("aaa"); ok {
		t.Fatal("aaa deberia estar fuera de la db")
	}
}

func TestManualFlag(t *testing.T) {
	root := t.TempDir()
	d, _ := db.Open(root)
	auto := false
	if err := Install(openPkg(t, buildPackage(t, "auto", nil, archive.Hooks{}, nil)), d, root, Options{Manual: &auto}); err != nil {
		t.Fatal(err)
	}
	if rec, _ := d.Get("auto"); rec.Manual {
		t.Fatal("el paquete deberia quedar marcado como auto")
	}
	if err := Install(openPkg(t, buildPackage(t, "world", nil, archive.Hooks{}, nil)), d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if rec, _ := d.Get("world"); !rec.Manual {
		t.Fatal("por defecto el paquete deberia quedar manual (world)")
	}
}

func TestUpgradeReplacesAndCleansStale(t *testing.T) {
	root := t.TempDir()
	d, _ := db.Open(root)
	v1 := buildPackage(t, "app", nil, archive.Hooks{}, map[string]string{"usr/lib/app.old": "obsoleto"})
	if err := Install(openPkg(t, v1), d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := Install(openPkg(t, v1), d, root, Options{}); err == nil {
		t.Fatal("reinstalar sin upgrade debe fallar")
	}

	old, _ := d.Get("app")
	v2 := buildPackageV(t, "app", "2.0.0", nil, archive.Hooks{}, map[string]string{"usr/lib/app.new": "nuevo"})
	if err := Upgrade(openPkg(t, v2), old, d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "usr/lib/app.old")); !os.IsNotExist(err) {
		t.Errorf("el archivo obsoleto deberia eliminarse: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "usr/lib/app.new")); err != nil {
		t.Errorf("faltaria el archivo nuevo: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "usr/bin/app")); err != nil {
		t.Errorf("el binario comun deberia seguir: %v", err)
	}
	rec, _ := d.Get("app")
	if rec.Pkgver != "2.0.0" || !rec.Manual {
		t.Errorf("el registro deberia actualizarse preservando manual: %+v", rec)
	}
}

func TestPruneOrphans(t *testing.T) {
	root := t.TempDir()
	d, _ := db.Open(root)
	auto := false
	// lib -> core, app -> lib; todo se instala auto salvo app
	if err := Install(openPkg(t, buildPackage(t, "core", nil, archive.Hooks{}, nil)), d, root, Options{Manual: &auto}); err != nil {
		t.Fatal(err)
	}
	lib := buildPackage(t, "lib", []string{"core"}, archive.Hooks{}, nil)
	if err := Install(openPkg(t, lib), d, root, Options{Manual: &auto}); err != nil {
		t.Fatal(err)
	}
	app := buildPackage(t, "app", []string{"lib"}, archive.Hooks{}, nil)
	if err := Install(openPkg(t, app), d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := RemovePackage("app", d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	// lib y core quedan instalados, sin dependientes: son huerfanos auto
	if err := PruneOrphans(d, root, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lib", "core"} {
		if _, ok := d.Get(name); ok {
			t.Errorf("%s deberia haber sido podado", name)
		}
	}
}
