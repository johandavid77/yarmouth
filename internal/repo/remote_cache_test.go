package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCachedPackagesCertifica: tras Sync+FetchPackage, la cache solo listo
// .yrm cuyo sha256 lo certifica un indice sincronizado. Un .yrm Huerfano
// escrito a mano en la cache (no passage por FetchPackage) jamas aparece:
// esa es la garantia auditable de import/export.
func TestCachedPackagesCertifica(t *testing.T) {
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
	// sync local sin red: AddRepo con ruta file://
	if err := AddRepo(root, "local", repoDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(root); err != nil {
		t.Fatal(err)
	}

	// poblar la cache REALMENTE: Sync solo baja el indice, es FetchPackage
	// quien escribe el .yrm fisico (verificando sha256 en el camino).
	m, err := Resolve(root, "foobar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FetchPackage(root, m); err != nil {
		t.Fatal(err)
	}

	// huerfano que SI entra a la cache por un atacante (le falta sha256 certificado)
	huerfano := filepath.Join(CacheRoot(root), "malvado-9.9.9-9.x86_64.yrm")
	if err := os.WriteFile(huerfano, []byte("basura que llego sin certificar"), 0o644); err != nil {
		t.Fatal(err)
	}

	cached, err := CachedPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	// 1) el bueno aparece, el huerfano NO
	if len(cached) != 1 {
		t.Fatalf("se esperaban solo los certificados (1), llego %d: %+v", len(cached), cached)
	}
	if cached[0].Remote.Alias != "local" || cached[0].Pkg.Filename() != p.Filename() {
		t.Fatalf("match inesperado: %+v", cached[0])
	}

	// 2) busca por sha256: el certificado resuelve, el huerfano no
	m, ok, err := LocalMatchBySHA(root, p.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || m.Pkg.SHA256 != p.SHA256 {
		t.Fatalf("LocalMatchBySHA certificado fallo: %+v ok=%v", m, ok)
	}
	if _, ok, _ := LocalMatchBySHA(root, strings.Repeat("0", 64)); ok {
		t.Fatal("una sha256 aleatoria no deberia certificar nada")
	}

	// 3) la cache no exporta el huerfano: verifica que CachedPackages solo
	//    reexporta lo que el indice firmado certifica (con sha igual al p)
	if got := cached[0].Pkg.SHA256; got != p.SHA256 {
		t.Fatalf("el sha256 exportado no coincide con el certificado: %s != %s", got, p.SHA256)
	}
}
