package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"yarmouth/internal/archive"
	"yarmouth/internal/repo"
	"yarmouth/internal/sig"
)

func runSign(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	key := fs.String("k", "", "archivo con la clave privada ed25519")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "firma un paquete .yrm (firma embebida) o un repodata (crea <repodata>.sig)")
		fmt.Fprintln(stderr, "\nUso: yarmouth sign -k <clave> <paquete.yrm|repodata>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *key == "" || fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	priv, err := sig.ReadPrivate(*key)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth sign: %v\n", err)
		return 1
	}
	target := fs.Arg(0)
	if strings.HasSuffix(target, ".yrm") {
		if err := archive.SignPackage(target, priv); err != nil {
			fmt.Fprintf(stderr, "yarmouth sign: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "firmado: %s\n", target)
		return 0
	}
	if err := repo.SignIndex(target, priv); err != nil {
		fmt.Fprintf(stderr, "yarmouth sign: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "firmado: %s -> %s.sig\n", target, target)
	return 0
}
