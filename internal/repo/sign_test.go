package repo

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncRequiresSignature(t *testing.T) {
	root := t.TempDir()
	repoDir := t.TempDir()
	if err := WriteIndexFile(filepath.Join(repoDir, "repodata"), []Package{
		{Name: "p", Version: "1.0", BuildID: "1", Arch: "x86_64", SHA256: "00", Size: 1},
	}); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hexPubSign(t, pub)

	// repositorio declarado con clave: sin repodata.sig no debe sincronizar
	if err := AddRepo(root, "local", repoDir, pubHex); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(root); err == nil {
		t.Fatal("un repo firmado sin repodata.sig deberia fallar")
	}
	if _, err := os.Stat(CacheIndex(root, "local")); !os.IsNotExist(err) {
		t.Fatal("no deberia haberse cacheado un indice no verificado")
	}

	// firmar el repodata y volver a sincronizar
	if err := SignIndex(filepath.Join(repoDir, "repodata"), priv); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(root); err != nil {
		t.Fatalf("con repodata.sig deberia sincronizar: %v", err)
	}

	// alterar la firma -> falla
	if err := os.WriteFile(filepath.Join(repoDir, "repodata.sig"), make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(root); err == nil {
		t.Fatal("una firma alterada deberia rechazarse")
	}

	// repositorio sin clave: se acepta sin firma (compatibilidad)
	root2 := t.TempDir()
	if err := AddRepo(root2, "local", repoDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(root2); err != nil {
		t.Fatalf("un repo sin clave deberia aceptar repodata sin firmar: %v", err)
	}
}

func TestRepoKeyRoundtrip(t *testing.T) {
	root := t.TempDir()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hexs := hexPubSign(t, pub)
	if err := AddRepo(root, "a", "/ruta/a", hexs); err != nil {
		t.Fatal(err)
	}
	remotes, err := ReadRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 1 || remotes[0].Alias != "a" || remotes[0].Key != hexs {
		t.Fatalf("roundtrip de la clave: %+v", remotes)
	}
}

func hexPubSign(t *testing.T, k ed25519.PublicKey) string {
	t.Helper()
	return hex.EncodeToString(k)
}
