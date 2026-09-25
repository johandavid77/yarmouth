package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"yarmouth/internal/db"
)

func runList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz de la base instalada (chroot)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "lista los paquetes instalados")
		fmt.Fprintln(stderr, "\nUso: yarmouth list -r <raiz> [patron]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}
	var pattern string
	if fs.NArg() == 1 {
		pattern = fs.Arg(0)
	}

	d, err := db.Open(*root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth list: %v\n", err)
		return 1
	}
	count := 0
	for _, r := range d.All() {
		if pattern != "" && !strings.Contains(r.Name, pattern) {
			continue
		}
		kind := "auto"
		if r.Manual {
			kind = "manual"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", r.Name, r.FullVersion(), r.Arch, kind)
		count++
	}
	if count == 0 {
		fmt.Fprintf(stderr, "yarmouth list: sin paquetes coincidentes\n")
		return 1
	}
	return 0
}
