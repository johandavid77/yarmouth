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
)

func runInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz donde se instala (chroot)")
	force := fs.Bool("f", false, "reemplazar archivos en conflicto")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "instala un paquete: por nombre desde los repositorios o desde un archivo .yrm")
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

	var pkg *archive.Package
	if looksLikeLocal(arg) {
		pkg, err = archive.Open(arg)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
	} else {
		m, err := repo.Resolve(*root, arg)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
		cached, err := repo.FetchPackage(*root, m)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "yarmouth install: descargado %s (%s)\n", m.Pkg.FullVersion(), m.Remote.Alias)
		pkg, err = archive.Open(cached)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
			return 1
		}
	}

	for _, dep := range pkg.Manifest.Depends {
		if _, ok := d.Get(dep); !ok {
			fmt.Fprintf(stderr, "aviso: %s depende de %q, que no esta instalado (resolucion de dependencias en fase 3)\n",
				pkg.Manifest.Pkgname, dep)
		}
	}

	if err := install.Install(pkg, d, *root, install.Options{Force: *force}); err != nil {
		fmt.Fprintf(stderr, "yarmouth install: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "instalado: %s-%s-%s (%s)\n",
		pkg.Manifest.Pkgname, pkg.Manifest.Pkgver, pkg.Manifest.BuildID, pkg.Manifest.Arch)
	return 0
}

func looksLikeLocal(arg string) bool {
	return strings.HasSuffix(arg, ".yrm")
}
