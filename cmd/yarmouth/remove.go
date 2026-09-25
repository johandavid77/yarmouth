package main

import (
	"flag"
	"fmt"
	"io"

	"yarmouth/internal/db"
	"yarmouth/internal/install"
)

func runRemove(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz de donde se desinstala (chroot)")
	force := fs.Bool("f", false, "forzar la baja aunque haya dependencias")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "desinstala un paquete del sistema (o de un chroot)")
		fmt.Fprintln(stderr, "\nUso: yarmouth remove -r <raiz> <paquete>")
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
		fmt.Fprintf(stderr, "yarmouth remove: %v\n", err)
		return 1
	}
	if err := install.RemovePackage(name, d, *root, install.Options{Force: *force}); err != nil {
		fmt.Fprintf(stderr, "yarmouth remove: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "eliminado: %s\n", name)
	return 0
}
