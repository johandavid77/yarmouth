package main

import (
	"flag"
	"fmt"
	"io"

	"yarmouth/internal/sig"
)

func runKeyCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("key", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "directorio raiz de la configuracion (chroot)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "administra las claves de confianza (trusted-keys)")
		fmt.Fprintln(stderr, "\nUso: yarmouth key -r <raiz> <add|del|list> ...")
		fmt.Fprintln(stderr, "  add <pub>   agrega una clave publica en hex")
		fmt.Fprintln(stderr, "  del <pub>   elimina una clave publica en hex")
		fmt.Fprintln(stderr, "  list        lista las claves de confianza")
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
		if len(rest) != 2 {
			fs.Usage()
			return 2
		}
		if err := sig.AddTrusted(*root, rest[1]); err != nil {
			fmt.Fprintf(stderr, "yarmouth key: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "clave agregada a trusted-keys\n")
	case "del":
		if len(rest) != 2 {
			fs.Usage()
			return 2
		}
		if err := sig.RemoveTrusted(*root, rest[1]); err != nil {
			fmt.Fprintf(stderr, "yarmouth key: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "clave eliminada de trusted-keys\n")
	case "list":
		hexs, err := sig.TrustedPublicHexes(*root)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth key: %v\n", err)
			return 1
		}
		if len(hexs) == 0 {
			fmt.Fprintln(stderr, "yarmouth key: sin claves de confianza")
			return 1
		}
		for _, h := range hexs {
			fmt.Fprintln(stdout, h)
		}
	default:
		fmt.Fprintf(stderr, "yarmouth key: subcomando desconocido %q\n", rest[0])
		fs.Usage()
		return 2
	}
	return 0
}
