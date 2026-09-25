package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"

	"yarmouth/internal/archive"
	"yarmouth/internal/db"
)

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz de la base instalada (chroot)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "verifica la integridad de los archivos de los paquetes instalados")
		fmt.Fprintln(stderr, "\nUso: yarmouth check -r <raiz>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	d, err := db.Open(*root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth check: %v\n", err)
		return 1
	}
	failed := 0
	total := 0
	for _, r := range d.All() {
		pkgOK := true
		for _, f := range r.Files {
			total++
			got, err := shaFile(archive.RootPath(*root, f.Path))
			if err != nil {
				fmt.Fprintf(stderr, "ERROR %s: %s\n", f.Path, err)
				pkgOK = false
				failed++
				continue
			}
			if got != f.SHA256 {
				fmt.Fprintf(stderr, "ERROR %s: hash %s != %s\n", f.Path, got, f.SHA256)
				pkgOK = false
				failed++
			}
		}
		for _, s := range r.Symlinks {
			total++
			if _, err := os.Lstat(archive.RootPath(*root, s.Path)); err != nil {
				fmt.Fprintf(stderr, "ERROR %s: %v\n", s.Path, err)
				pkgOK = false
				failed++
			}
		}
		for _, dir := range r.Dirs {
			total++
			fi, err := os.Lstat(archive.RootPath(*root, dir))
			if err != nil || !fi.IsDir() {
				fmt.Fprintf(stderr, "ERROR %s: falta el directorio %q\n", r.Name, dir)
				pkgOK = false
				failed++
			}
		}
		if pkgOK {
			fmt.Fprintf(stdout, "ok        %s\n", r.Name)
		} else {
			fmt.Fprintf(stderr, "FALLO     %s\n", r.Name)
		}
	}
	if failed > 0 {
		fmt.Fprintf(stderr, "yarmouth check: %d/%d archivos o enlaces con problemas\n", failed, total)
		return 1
	}
	fmt.Fprintf(stdout, "integridad correcta: %d paquetes, %d archivos/enlaces\n", len(d.All()), total)
	return 0
}

func shaFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
