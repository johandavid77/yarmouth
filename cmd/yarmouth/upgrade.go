package main

import (
	"flag"
	"fmt"
	"io"

	"yarmouth/internal/db"
	"yarmouth/internal/resolve"
)

func runUpgrade(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz (chroot)")
	force := fs.Bool("f", false, "reemplazar archivos en conflicto")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "actualiza a la version mas reciente los paquetes indicados (o todos los instalados)")
		fmt.Fprintln(stderr, "\nUso: yarmouth upgrade -r <raiz> [paquete...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	d, err := db.Open(*root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth upgrade: %v\n", err)
		return 1
	}
	entries, skipped, err := resolve.UpgradePlan(*root, d, fs.Args(), fs.NArg() > 0)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth upgrade: %v\n", err)
		return 1
	}
	for _, s := range skipped {
		fmt.Fprintf(stderr, "yarmouth upgrade: %s\n", s)
	}
	if len(entries) == 0 {
		if len(skipped) == 0 {
			fmt.Fprintln(stderr, "yarmouth upgrade: nada mas reciente en los repositorios")
		}
		return 0
	}
	return commitPlan(*root, d, entries, *force, stdout, stderr)
}
