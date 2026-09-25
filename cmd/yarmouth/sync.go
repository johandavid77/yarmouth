package main

import (
	"flag"
	"fmt"
	"io"

	"yarmouth/internal/repo"
)

func runSync(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz donde se guarda la cache (chroot)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "descarga los indices de los repositorios configurados")
		fmt.Fprintln(stderr, "\nUso: yarmouth sync -r <raiz>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "yarmouth sync: argumentos de mas: %v\n", fs.Args())
		return 2
	}

	remotes, err := repo.Sync(*root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth sync: %v\n", err)
		return 1
	}
	for _, r := range remotes {
		index, err := repo.ReadIndexFile(repo.CacheIndex(*root, r.Alias))
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth sync: %q: %v\n", r.Alias, err)
			continue
		}
		fmt.Fprintf(stdout, "%s\t%s\t%d paquetes\n", r.Alias, r.URL, len(index))
	}
	return 0
}
