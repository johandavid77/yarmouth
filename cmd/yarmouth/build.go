package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yarmouth/internal/archive"
	"yarmouth/internal/metadata"
)

func runBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifest := fs.String("manifest", "", "manifest del paquete (INI) con pkgname, pkgver, buildid y arch")
	destdir := fs.String("destdir", "", "directorio con los archivos a empaquetar (DESTDIR)")
	out := fs.String("out", ".", "directorio donde escribir el paquete")
	preInstall := fs.String("pre-install", "", "hook opcional que se ejecuta antes de instalar")
	postInstall := fs.String("post-install", "", "hook opcional que se ejecuta despues de instalar")
	preRemove := fs.String("pre-remove", "", "hook opcional que se ejecuta antes de desinstalar")
	postRemove := fs.String("post-remove", "", "hook opcional que se ejecuta despues de desinstalar")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "construye un paquete .yrm desde un DESTDIR")
		fmt.Fprintln(stderr, "\nUso: yarmouth build -manifest <manifest> -destdir <dir> [opciones]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "yarmouth build: argumentos de mas: %v\n", fs.Args())
		return 2
	}
	if *manifest == "" || *destdir == "" {
		fs.Usage()
		return 2
	}

	mf, err := metadata.ReadFile(*manifest)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth build: %v\n", err)
		return 1
	}
	if len(mf.Files) > 0 || mf.DataHash != "" {
		fmt.Fprintln(stderr, "yarmouth build: el manifest no debe declarar file ni datahash")
		return 1
	}

	hooks, err := loadHooks(*preInstall, *postInstall, *preRemove, *postRemove)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth build: %v\n", err)
		return 1
	}

	name := fmt.Sprintf("%s-%s-%s.%s.yrm", mf.Pkgname, mf.Pkgver, mf.BuildID, mf.Arch)
	dst := filepath.Join(*out, name)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(stderr, "yarmouth build: %v\n", err)
		return 1
	}
	f, err := os.Create(dst)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth build: %v\n", err)
		return 1
	}
	res, err := archive.Create(f, archive.CreateOptions{Manifest: mf, DestDir: *destdir, Hooks: hooks})
	cerr := f.Close()
	if err != nil {
		os.Remove(dst)
		fmt.Fprintf(stderr, "yarmouth build: %v\n", err)
		return 1
	}
	if cerr != nil {
		fmt.Fprintf(stderr, "yarmouth build: %v\n", cerr)
		return 1
	}
	fmt.Fprintf(stdout, "%s\n  archivos: %d\n  datahash: %s\n", name, len(res.Files), res.DataHash)
	return 0
}

func loadHooks(preI, postI, preR, postR string) (archive.Hooks, error) {
	var h archive.Hooks
	var err error
	if h.PreInstall, err = readHook(preI); err != nil {
		return h, err
	}
	if h.PostInstall, err = readHook(postI); err != nil {
		return h, err
	}
	if h.PreRemove, err = readHook(preR); err != nil {
		return h, err
	}
	if h.PostRemove, err = readHook(postR); err != nil {
		return h, err
	}
	return h, nil
}

func readHook(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	return os.ReadFile(path)
}
