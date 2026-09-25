package repo

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestWriteReadRoundtrip(t *testing.T) {
	pkgs := []Package{
		{Name: "zlib", Version: "1.3.1", BuildID: "1", Arch: "x86_64", SHA256: strings.Repeat("a", 64), Size: 100, Depends: []string{"glibc"}},
		{Name: "hello", Version: "1.0", BuildID: "2", Arch: "x86_64", SHA256: strings.Repeat("b", 64), Size: 200},
		{Name: "hello", Version: "1.0", BuildID: "1", Arch: "x86_64", SHA256: strings.Repeat("c", 64), Size: 300},
	}
	var buf bytes.Buffer
	if err := WriteIndex(&buf, pkgs); err != nil {
		t.Fatal(err)
	}
	got, err := ReadIndex(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("paquetes = %d", len(got))
	}
	if got[0].Name != "hello" || got[0].BuildID != "1" {
		t.Errorf("orden incorrecto: %+v", got[0])
	}
	if got[1].BuildID != "2" {
		t.Errorf("orden incorrecto: %+v", got[1])
	}
	if !reflect.DeepEqual(got[2].Depends, []string{"glibc"}) {
		t.Errorf("depends: %v", got[2].Depends)
	}
}

func TestFilename(t *testing.T) {
	p := Package{Name: "hello", Version: "1.0", BuildID: "1", Arch: "x86_64"}
	if p.Filename() != "hello-1.0-1.x86_64.yrm" {
		t.Errorf("filename = %q", p.Filename())
	}
}
