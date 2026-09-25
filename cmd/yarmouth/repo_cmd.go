package main

import (
	"flag"
	"fmt"
	"io"

	"yarmouth/internal/repo"
)

func runRepoCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("repo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz de la configuracion (chroot)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "administra los repositorios de paquetes")
		fmt.Fprintln(stderr, "\nUso: yarmouth repo -r <raiz> <add|del|list> ...")
		fmt.Fprintln(stderr, "  add <alias> <url>   agrega un repositorio (URL http(s) o ruta local)")
		fmt.Fprintln(stderr, "  del <alias>         elimina un repositorio")
		fmt.Fprintln(stderr, "  list                lista los repositorios configurados")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	switch rest[0] {
	case "add":
		if len(rest) != 3 {
			fs.Usage()
			return 2
		}
		if err := repo.AddRepo(*root, rest[1], rest[2]); err != nil {
			fmt.Fprintf(stderr, "yarmouth repo: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "repositorio agregado: %s -> %s\n", rest[1], rest[2])
	case "del":
		if len(rest) != 2 {
			fs.Usage()
			return 2
		}
		if err := repo.RemoveRepo(*root, rest[1]); err != nil {
			fmt.Fprintf(stderr, "yarmouth repo: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "repositorio eliminado: %s\n", rest[1])
	case "list":
		remotes, err := repo.ReadRepos(*root)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth repo: %v\n", err)
			return 1
		}
		if len(remotes) == 0 {
			fmt.Fprintln(stderr, "yarmouth repo: sin repositorios configurados")
			return 1
		}
		for _, r := range remotes {
			fmt.Fprintf(stdout, "%s\t%s\n", r.Alias, r.URL)
		}
	default:
		fmt.Fprintf(stderr, "yarmouth repo: subcomando desconocido %q\n", rest[0])
		fs.Usage()
		return 2
	}
	return 0
}
