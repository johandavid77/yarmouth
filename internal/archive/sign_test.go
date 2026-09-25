package archive

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"os"
	"testing"

	"github.com/ulikunitz/xz"
	"yarmouth/internal/metadata"
)

func buildSigned(t *testing.T) (string, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
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
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := SignPackage(out, priv); err != nil {
		t.Fatal(err)
	}
	return out, pub, priv
}

func TestSignAndVerify(t *testing.T) {
	out, pub, priv := buildSigned(t)
	p, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Sig) == 0 {
		t.Fatal("no se capturo la firma embebida")
	}
	if !p.VerifySig(pub) {
		t.Fatal("la firma deberia verificar con la clave del firmante")
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if p.VerifySig(other) {
		t.Fatal("una clave ajena no deberia verificar")
	}
	if err := p.VerifyContent(); err != nil {
		t.Fatalf("VerifyContent falla tras firmar: %v", err)
	}
	if err := SignPackage(out, priv); err == nil {
		t.Fatal("no deberia permitirse firmar dos veces")
	}
	// la entrada de firma debe quedar fuera del payload y ser control
	for _, e := range p.Entries {
		if e.Name == SigPath {
			t.Fatal("la firma no deberia listarse como entrada de contenido")
		}
	}
}

func TestTamperInvalidatesFirma(t *testing.T) {
	out, pub, _ := buildSigned(t)
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
	idx := bytes.Index(raw, []byte("datahash = "))
	if idx < 0 {
		t.Fatal("no se encontro datahash en el manifest")
	}
	// cambiar un hex del datahash: el manifest se lee bien pero la firma ya no vale
	pos := idx + len("datahash = ") + 2
	raw[pos] ^= 0xFF
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
	p, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if p.VerifySig(pub) {
		t.Fatal("la firma deberia invalidarse al alterar el datahash")
	}
}
