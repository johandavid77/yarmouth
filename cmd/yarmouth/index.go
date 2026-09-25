package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yarmouth/internal/archive"
	"yarmouth/internal/repo"
)

func runIndex(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "directorio del repositorio con los paquetes .yrm")
	out := fs.String("out", "repodata", "archivo de indice de salida")
	arch := fs.String("arch", "", "filtrar paquetes por arquitectura (vacio = todas)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "genera el indice de un repositorio local")
		fmt.Fprintln(stderr, "\nUso: yarmouth index -dir <dir> [-out <repodata>] [-arch <arch>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "yarmouth index: argumentos de mas: %v\n", fs.Args())
		return 2
	}

	matches, err := filepath.Glob(filepath.Join(*dir, "*.yrm"))
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth index: %v\n", err)
		return 1
	}
	if len(matches) == 0 {
		fmt.Fprintf(stderr, "yarmouth index: no hay paquetes .yrm en %q\n", *dir)
		return 1
	}

	var pkgs []repo.Package
	for _, m := range matches {
		p, err := archive.Open(m)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth index: %v\n", err)
			return 1
		}
		if *arch != "" && p.Manifest.Arch != *arch {
			continue
		}
		sh, size, err := sha256File(m)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth index: %v\n", err)
			return 1
		}
		pkgs = append(pkgs, repo.Package{
			Name:    p.Manifest.Pkgname,
			Version: p.Manifest.Pkgver,
			BuildID: p.Manifest.BuildID,
			Arch:    p.Manifest.Arch,
			SHA256:  sh,
			Size:    size,
			Depends: p.Manifest.Depends,
		})
	}

	var buf bytes.Buffer
	if err := repo.WriteIndex(&buf, pkgs); err != nil {
		fmt.Fprintf(stderr, "yarmouth index: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(stderr, "yarmouth index: %v\n", err)
		return 1
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(stderr, "yarmouth index: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "repodata: %d paquetes -> %s\n", len(pkgs), *out)
	return 0
}

func sha256File(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
