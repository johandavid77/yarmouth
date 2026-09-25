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
	list := fs.Bool("l", false, "lista las actualizaciones pendientes sin aplicarlas")
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
	if *list {
		return listUpgradePlan(stdout, d, entries)
	}
	if len(entries) == 0 {
		if len(skipped) == 0 {
			fmt.Fprintln(stderr, "yarmouth upgrade: nada mas reciente en los repositorios")
		}
		return 0
	}
	return commitPlan(*root, d, entries, *force, stdout, stderr)
}

func listUpgradePlan(stdout io.Writer, d *db.DB, entries []resolve.Entry) int {
	ups, news := 0, 0
	for _, e := range entries {
		if e.Upgrade {
			old, _ := d.Get(e.Name)
			fmt.Fprintf(stdout, "actualizable: %s %s -> %s\n", e.Name, old.FullVersion(), e.Version)
			ups++
		} else {
			fmt.Fprintf(stdout, "nueva dependencia (auto): %s %s\n", e.Name, e.Version)
			news++
		}
	}
	if ups+news == 0 {
		fmt.Fprintln(stdout, "sistema actualizado")
		return 0
	}
	fmt.Fprintf(stdout, "%d actualizaciones, %d nuevas dependencias\n", ups, news)
	return 0
}
