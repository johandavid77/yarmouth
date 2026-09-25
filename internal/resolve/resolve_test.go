package resolve

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"yarmouth/internal/archive"
	"yarmouth/internal/db"
	"yarmouth/internal/metadata"
	"yarmouth/internal/repo"
)

type testPkg struct {
	name string
	ver  string
	deps []string
}

func buildReposForTest(t *testing.T, pkgs []testPkg) string {
	t.Helper()
	root := t.TempDir()
	repoDir := t.TempDir()
	var indexed []repo.Package
	for _, p := range pkgs {
		m := metadata.Manifest{Pkgname: p.name, Pkgver: p.ver, BuildID: "1", Arch: "x86_64", Depends: p.deps}
		dest := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dest, "usr", "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "usr", "bin", p.name), []byte(p.name+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(repoDir, p.name+"-"+p.ver+"-1.x86_64.yrm")
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Create(f, archive.CreateOptions{Manifest: m, DestDir: dest}); err != nil {
			t.Fatal(err)
		}
		f.Close()
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		fi, err := os.Stat(out)
		if err != nil {
			t.Fatal(err)
		}
		indexed = append(indexed, repo.Package{
			Name: p.name, Version: p.ver, BuildID: "1", Arch: "x86_64",
			SHA256: hex.EncodeToString(sum[:]), Size: fi.Size(), Depends: p.deps,
		})
	}
	if err := repo.WriteIndexFile(filepath.Join(repoDir, "repodata"), indexed); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddRepo(root, "local", repoDir); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Sync(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func addInstalled(t *testing.T, root, name string, manual bool) {
	t.Helper()
	d, err := db.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d.Add(&db.Record{Name: name, Pkgver: "0.0.1", BuildID: "1", Manual: manual, Hardlinks: map[string]string{}, Hooks: map[string][]byte{}})
}

func names(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func TestPlanDepsFirst(t *testing.T) {
	root := buildReposForTest(t, []testPkg{
		{"a", "1.0", []string{"b"}},
		{"b", "1.0", []string{"c"}},
		{"c", "1.0", nil},
	})
	d, _ := db.Open(root)
	m, err := repo.Resolve(root, "a")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := Plan(root, d, []Head{{Name: "a", Match: m, Manual: true}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"c", "b", "a"}
	got := names(entries)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("orden: got %v, want %v", got, want)
		}
	}
	if !entries[2].Manual || entries[0].Manual || entries[1].Manual {
		t.Errorf("marcas manual: %+v", entries)
	}
}

func TestPlanSatisfiedInstalled(t *testing.T) {
	root := buildReposForTest(t, []testPkg{
		{"a", "1.0", []string{"b"}},
		{"b", "1.0", nil},
	})
	addInstalled(t, root, "b", true)
	d, _ := db.Open(root)
	m, _ := repo.Resolve(root, "a")
	entries, err := Plan(root, d, []Head{{Name: "a", Match: m, Manual: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(entries); len(got) != 1 || got[0] != "a" {
		t.Fatalf("no deberia reinstalar b instalado: %v", got)
	}
}

func TestPlanCycle(t *testing.T) {
	root := buildReposForTest(t, []testPkg{
		{"a", "1.0", []string{"b"}},
		{"b", "1.0", []string{"a"}},
	})
	d, _ := db.Open(root)
	m, _ := repo.Resolve(root, "a")
	entries, err := Plan(root, d, []Head{{Name: "a", Match: m, Manual: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(entries); len(got) != 2 {
		t.Fatalf("ciclo deberia producir 2 entradas, got %v", got)
	}
}

func TestPlanMissingDep(t *testing.T) {
	root := buildReposForTest(t, []testPkg{
		{"a", "1.0", []string{"ghost"}},
	})
	d, _ := db.Open(root)
	m, _ := repo.Resolve(root, "a")
	if _, err := Plan(root, d, []Head{{Name: "a", Match: m, Manual: true}}); err == nil {
		t.Fatal("se esperaba error por dependencia ausente en los repos")
	}
}

func TestUpgradePlan(t *testing.T) {
	root := buildReposForTest(t, []testPkg{
		{"p", "1.0.0", nil},
		{"p", "2.0.0", []string{"q"}},
		{"q", "1.0", nil},
	})
	addInstalled(t, root, "p", true)
	d, _ := db.Open(root)
	entries, skipped, err := UpgradePlan(root, d, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped: %v", skipped)
	}
	got := names(entries)
	if len(got) != 2 || got[0] != "q" || got[1] != "p" {
		t.Fatalf("plan de upgrade: %v", got)
	}
	if !entries[1].Upgrade || entries[0].Upgrade {
		t.Errorf("marcas upgrade: %+v", entries)
	}
	if entries[0].Manual {
		t.Error("q deberia quedar como auto")
	}
	if !entries[1].Manual {
		t.Error("p preserva su marca manual")
	}
}

func TestUpgradePlanExplicitMissing(t *testing.T) {
	root := buildReposForTest(t, nil)
	addInstalled(t, root, "foo", true)
	d, _ := db.Open(root)
	if _, _, err := UpgradePlan(root, d, []string{"foo"}, true); err == nil {
		t.Fatal("upgrade explicito de un paquete sin repo deberia fallar")
	}
}
