package main

import (
	"fmt"
	"io"
	"os"
)

const version = "0.3.0"

type command struct {
	name  string
	short string
	usage string
	run   func(args []string, stdout, stderr io.Writer) int
}

var commands = []command{
	{
		name:  "build",
		short: "construye un paquete .yrm desde un DESTDIR",
		usage: "yarmouth build -manifest <manifest> -destdir <dir> [opciones]",
		run:   runBuild,
	},
	{
		name:  "index",
		short: "genera el indice de un repositorio local",
		usage: "yarmouth index -dir <dir> [-out <repodata>] [-arch <arch>]",
		run:   runIndex,
	},
	{
		name:  "repo",
		short: "administra repositorios (add/del/list)",
		usage: "yarmouth repo -r <raiz> <add|del|list> ...",
		run:   runRepoCmd,
	},
	{
		name:  "sync",
		short: "descarga los indices de los repositorios",
		usage: "yarmouth sync -r <raiz>",
		run:   runSync,
	},
	{
		name:  "install",
		short: "instala un paquete (por nombre o .yrm)",
		usage: "yarmouth install -r <raiz> <paquete|paquete.yrm>",
		run:   runInstall,
	},
	{
		name:  "remove",
		short: "desinstala un paquete",
		usage: "yarmouth remove -r <raiz> [-o] <paquete>",
		run:   runRemove,
	},
	{
		name:  "upgrade",
		short: "actualiza los paquetes instalados",
		usage: "yarmouth upgrade -r <raiz> [paquete...]",
		run:   runUpgrade,
	},
	{
		name:  "list",
		short: "lista los paquetes instalados",
		usage: "yarmouth list -r <raiz> [patron]",
		run:   runList,
	},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) > 1 {
			printCmdHelp(stderr, args[1])
		} else {
			usage(stderr)
		}
		return 0
	case "-V", "--version":
		fmt.Fprintf(stdout, "yarmouth %s\n", version)
		return 0
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "yarmouth: comando desconocido %q\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "yarmouth %s — gestion de paquetes para sistemas LFS\n\nUso:\n  yarmouth <comando> [opciones]\n\nComandos:\n", version)
	for _, c := range commands {
		fmt.Fprintf(w, "  %-9s %s\n", c.name, c.short)
	}
	fmt.Fprintf(w, "  %-9s %s\n", "help", "muestra ayuda (yarmouth help <comando>)")
	fmt.Fprintf(w, "\nOpciones globales:\n  -h, --help      muestra esta ayuda\n  -V, --version   muestra la version\n")
}

func printCmdHelp(w io.Writer, name string) {
	for _, c := range commands {
		if c.name == name {
			fmt.Fprintf(w, "%s — %s\n\nUso:\n  %s\n", c.name, c.short, c.usage)
			return
		}
	}
	fmt.Fprintf(w, "yarmouth: no hay ayuda para %q\n", name)
}
