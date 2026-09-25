package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"yarmouth/internal/archive"
	"yarmouth/internal/db"
	"yarmouth/internal/install"
	"yarmouth/internal/repo"
	"yarmouth/internal/resolve"
	"yarmouth/internal/sig"
)

func runInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz donde se instala (chroot)")
	force := fs.Bool("f", false, "reemplazar archivos en conflicto")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "instala un paquete y sus dependencias: por nombre desde los repositorios o desde un .yrm")
		fmt.Fprintln(stderr, "\nUso: yarmouth install -r <raiz> <paquete|paquete.yrm>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	arg := fs.Arg(0)

	d, err := db.Open(*root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
		return 1
	}

	var entries []resolve.Entry
	if looksLikeLocal(arg) {
		pk, err := archive.Open(arg)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
		if _, ok := d.Get(pk.Manifest.Pkgname); ok {
			fmt.Fprintf(stderr, "yarmouth install: el paquete ya esta instalado\n")
			return 1
		}
		entries, err = resolve.Plan(*root, d, []resolve.Head{{Name: pk.Manifest.Pkgname, Local: arg, Manual: true}})
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
	} else {
		if _, ok := d.Get(arg); ok {
			fmt.Fprintf(stderr, "yarmouth install: el paquete %q ya esta instalado (usa yarmouth upgrade)\n", arg)
			return 1
		}
		m, err := repo.Resolve(*root, arg)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
		entries, err = resolve.Plan(*root, d, []resolve.Head{{Name: arg, Match: m, Manual: true}})
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
	}

	return commitPlan(*root, d, entries, *force, stdout, stderr)
}

func looksLikeLocal(arg string) bool {
	return strings.HasSuffix(arg, ".yrm")
}

func verifyPackageTrust(pkg *archive.Package, root string) error {
	keys, err := sig.LoadTrusted(root)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	if len(pkg.Sig) == 0 {
		return fmt.Errorf("paquete %q sin firmar (hay claves de confianza configuradas)", pkg.Manifest.Pkgname)
	}
	for _, k := range keys {
		if pkg.VerifySig(k) {
			return nil
		}
	}
	return fmt.Errorf("firma de %q no verificada por ninguna clave de confianza", pkg.Manifest.Pkgname)
}

func commitPlan(root string, d *db.DB, entries []resolve.Entry, force bool, stdout, stderr io.Writer) int {
	for _, e := range entries {
		src := e.Local
		if src == "" {
			cached, err := repo.FetchPackage(root, e.Match)
			if err != nil {
				fmt.Fprintf(stderr, "yarmouth: %v\n", err)
				return 1
			}
			src = cached
		}
		pkg, err := archive.Open(src)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth: %v\n", err)
			return 1
		}
		if err := verifyPackageTrust(pkg, root); err != nil {
			fmt.Fprintf(stderr, "yarmouth: %v\n", err)
			return 1
		}
		opts := install.Options{Force: force}
		if e.Upgrade {
			old, _ := d.Get(e.Name)
			err = install.Upgrade(pkg, old, d, root, opts)
		} else {
			m := e.Manual
			opts.Manual = &m
			err = install.Install(pkg, d, root, opts)
		}
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth: %v\n", err)
			return 1
		}
		if e.Upgrade {
			fmt.Fprintf(stdout, "actualizado: %s (%s)\n", e.Name, e.Version)
		} else {
			fmt.Fprintf(stdout, "instalado: %s-%s (%s)\n", pkg.Manifest.Pkgname, e.Version, pkg.Manifest.Arch)
		}
	}
	return 0
}
