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
		fmt.Fprintln(stderr, "  add [opciones] <alias> <url>  agrega un repositorio")
		fmt.Fprintln(stderr, "        -k <pub>  clave ed25519 (hex) que debe firmar el repodata")
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
		subs := flag.NewFlagSet("repo add", flag.ContinueOnError)
		subs.SetOutput(stderr)
		key := subs.String("k", "", "clave publica ed25519 (hex) que firma el repodata")
		if err := subs.Parse(rest[1:]); err != nil {
			return 2
		}
		if subs.NArg() != 2 {
			fs.Usage()
			return 2
		}
		alias, url := subs.Arg(0), subs.Arg(1)
		if err := repo.AddRepo(*root, alias, url, *key); err != nil {
			fmt.Fprintf(stderr, "yarmouth repo: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "repositorio agregado: %s -> %s\n", alias, url)
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
			fmt.Fprintf(stdout, "%s\t%s", r.Alias, r.URL)
			if r.Key != "" {
				fmt.Fprintf(stdout, "\t(clave %s)", r.Key)
			}
			fmt.Fprintln(stdout)
		}
	default:
		fmt.Fprintf(stderr, "yarmouth repo: subcomando desconocido %q\n", rest[0])
		fs.Usage()
		return 2
	}
	return 0
}
