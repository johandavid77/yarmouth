package sig

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAndReadPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.key")
	pub, err := Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != ed25519.PublicKeySize*2 {
		t.Fatalf("clave publica de tamano invalido: %d", len(pub))
	}
	priv, err := ReadPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	if PublicHex(priv) != pub {
		t.Fatal("la clave publica derivada no coincide")
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("la clave privada deberia quedar con modo 0600: %v", fi.Mode())
	}
}

func TestTrustedKeysRoundtrip(t *testing.T) {
	root := t.TempDir()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyHex := hex.EncodeToString(pub)
	if err := AddTrusted(root, keyHex); err != nil {
		t.Fatal(err)
	}
	it, err := TrustedPublicHexes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(it) != 1 || it[0] != keyHex {
		t.Fatalf("keyring: %v", it)
	}
	if err := AddTrusted(root, keyHex); err == nil {
		t.Fatal("duplicar una clave debe fallar")
	}
	if err := RemoveTrusted(root, keyHex); err != nil {
		t.Fatal(err)
	}
	if it2, _ := TrustedPublicHexes(root); len(it2) != 0 {
		t.Fatalf("keyring deberia quedar vacio: %v", it2)
	}
	if err := RemoveTrusted(root, keyHex); err == nil {
		t.Fatal("quitar una clave inexistente debe fallar")
	}
	if err := AddTrusted(root, "zz"); err == nil {
		t.Fatal("una clave publica invalida debe fallar")
	}
}
