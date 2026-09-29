package main

import (
	"flag"
	"fmt"
	"io"

	"yarmouth/internal/sig"
)

func runPubkey(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pubkey", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "imprime la clave publica en hex de una clave privada")
		fmt.Fprintln(stderr, "\nUso: yarmouth pubkey <clave-privada>")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	priv, err := sig.ReadPrivate(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth pubkey: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, sig.PublicHex(priv))
	return 0
}

func init() {
	commands = append(commands, command{
		name:    "pubkey",
		short:   "imprime la clave publica de una clave privada",
		usage:   "yarmouth pubkey <clave-privada>",
		details: "lee la clave privada (hex) generada por keygen e imprime su clave publica en hex, la que se distribuye con repo add -k y key add.",
		run:     runPubkey,
	})
}
