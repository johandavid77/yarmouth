package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"yarmouth/internal/db"
)

func runQuery(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz de la base instalada (chroot)")
	files := fs.Bool("f", false, "mostrar tambien los archivos del paquete")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "muestra los detalles de un paquete instalado")
		fmt.Fprintln(stderr, "\nUso: yarmouth query -r <raiz> [-f] <paquete>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	name := fs.Arg(0)

	d, err := db.Open(*root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth query: %v\n", err)
		return 1
	}
	rec, ok := d.Get(name)
	if !ok {
		fmt.Fprintf(stderr, "yarmouth query: paquete %q no esta instalado\n", name)
		return 1
	}
	kind := "auto"
	if rec.Manual {
		kind = "manual"
	}
	fmt.Fprintf(stdout, "paquete    %s\n", rec.Name)
	fmt.Fprintf(stdout, "version    %s\n", rec.FullVersion())
	fmt.Fprintf(stdout, "arch       %s\n", rec.Arch)
	fmt.Fprintf(stdout, "marca      %s\n", kind)
	fmt.Fprintf(stdout, "sha256     %s\n", rec.SHA256)
	fmt.Fprintf(stdout, "datahash   %s\n", rec.DataHash)
	fmt.Fprintf(stdout, "depende de %s\n", strings.Join(rec.Depends, ", "))
	hooks := "ninguno"
	if len(rec.Hooks) > 0 {
		names := make([]string, 0, len(rec.Hooks))
		for h := range rec.Hooks {
			names = append(names, strings.TrimPrefix(h, "yarmouth/"))
		}
		hooks = strings.Join(names, ", ")
	}
	fmt.Fprintf(stdout, "hooks      %s\n", hooks)
	fmt.Fprintf(stdout, "archivos   %d, symlinks %d, direc. %d, hardlinks %d\n",
		len(rec.Files), len(rec.Symlinks), len(rec.Dirs), len(rec.Hardlinks))
	if *files {
		for _, f := range rec.Files {
			fmt.Fprintf(stdout, "  %s  %s\n", f.SHA256, f.Path)
		}
	}
	return 0
}
