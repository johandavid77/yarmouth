package main

import (
	"flag"
	"fmt"
	"io"

	"yarmouth/internal/sig"
)

func runKeygen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "yarmouth.key", "archivo donde guardar la clave privada (hex)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "genera un par de claves ed25519 para firmar paquetes e indices")
		fmt.Fprintln(stderr, "\nUso: yarmouth keygen [-out <archivo>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}
	pub, err := sig.Generate(*out)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth keygen: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "clave guardada en %s\n", *out)
	fmt.Fprintf(stdout, "clave publica:\n%s\n", pub)
	return 0
}
