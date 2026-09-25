package sig

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"yarmouth/internal/archive"
)

const trustedFile = "etc/yarmouth/trusted-keys"

// Generate crea un par de claves ed25519 y lo guarda (privada en hex) en out.
// Devuelve la clave publica en hex.
func Generate(out string) (string, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dirOf(out), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(out, []byte(hex.EncodeToString(priv)+"\n"), 0o600); err != nil {
		return "", err
	}
	return hexPub(pub), nil
}

func dirOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}

func ReadPrivate(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(data))
	b, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("clave privada invalida en %s: %w", path, err)
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("clave privada en %s: esperaba %d bytes, hay %d", path, ed25519.PrivateKeySize, len(b))
	}
	return ed25519.PrivateKey(b), nil
}

func PublicHex(priv ed25519.PrivateKey) string {
	return hexPub(priv.Public().(ed25519.PublicKey))
}

func ParsePublic(s string) (ed25519.PublicKey, error) {
	raw := strings.TrimSpace(s)
	b, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("clave publica invalida: %w", err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("clave publica: esperaba %d bytes, hay %d", ed25519.PublicKeySize, len(b))
	}
	return ed25519.PublicKey(b), nil
}

func TrustedPath(root string) string { return archive.RootPath(root, trustedFile) }

func LoadTrusted(root string) ([]ed25519.PublicKey, error) {
	data, err := os.ReadFile(TrustedPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []ed25519.PublicKey
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) != ed25519.PublicKeySize*2 {
			return nil, fmt.Errorf("clave publica invalida en trusted-keys: %q", line)
		}
		k, err := ParsePublic(line)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func AddTrusted(root, pubHex string) error {
	k, err := ParsePublic(pubHex)
	if err != nil {
		return err
	}
	keys, err := LoadTrusted(root)
	if err != nil {
		return err
	}
	hexs := make([]string, 0, len(keys)+1)
	hexs = append(hexs, hexPub(k))
	for _, existing := range keys {
		h := hexPub(existing)
		if h == pubHex {
			return fmt.Errorf("la clave ya esta en trusted-keys")
		}
		hexs = append(hexs, h)
	}
	return writeTrusted(root, hexs)
}

func RemoveTrusted(root, pubHex string) error {
	if _, err := ParsePublic(pubHex); err != nil {
		return err
	}
	keys, err := LoadTrusted(root)
	if err != nil {
		return err
	}
	hexs := make([]string, 0, len(keys))
	found := false
	for _, k := range keys {
		h := hexPub(k)
		if h == pubHex {
			found = true
			continue
		}
		hexs = append(hexs, h)
	}
	if !found {
		return fmt.Errorf("la clave no esta en trusted-keys")
	}
	return writeTrusted(root, hexs)
}

func TrustedPublicHexes(root string) ([]string, error) {
	keys, err := LoadTrusted(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = hexPub(k)
	}
	return out, nil
}

func writeTrusted(root string, hexs []string) error {
	var b strings.Builder
	b.WriteString("# claves ed25519 de confianza (yarmouth)\n")
	for _, h := range hexs {
		b.WriteString(h + "\n")
	}
	if err := os.MkdirAll(dirOf(TrustedPath(root)), 0o755); err != nil {
		return err
	}
	tmp := TrustedPath(root) + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, TrustedPath(root)); err != nil {
		return err
	}
	return nil
}

func hexPub(k ed25519.PublicKey) string { return hex.EncodeToString(k) }
