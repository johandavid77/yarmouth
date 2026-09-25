package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yarmouth/internal/repo"
)

func init() {
	commands = append(commands, command{
		name:    "cache",
		short:   "gestiona la cache local de .yrm (export/import/list)",
		usage:   "yarmouth cache -r <raiz> <export|import|list> ...",
		details: "opera sobre la cache de paquetes descargados (raiz/var/cache/yarmouth) de forma segura y auditable: NUNCA se copia un .yrm hacia fuera (export) o hacia dentro (import) sin que su sha256 este certificado por un indice firmado local (sync/verify). import solo acepta .yrm cuyo sha256 coincida con un indice sincronizado y firmado; un .yrm huerfano o alterado se rechaza y se reporta. list muestra la cache actual con su sha256 verificado.",
		run:     runCacheCmd,
	})
}

func runCacheCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("r", "/", "raiz (directorio base de la configuracion)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "yarmouth cache: exporta/importa/lista la cache local de .yrm")
		fmt.Fprintln(stderr, "  yarmouth cache -r <raiz> export -out <dir>     copia a <dir> solo los .yrm certificados")
		fmt.Fprintln(stderr, "  yarmouth cache -r <raiz> import -dir <dir>    copia a la cache solo los .yrm cuyo")
		fmt.Fprintln(stderr, "                                               sha256 certifica un indice firmado local")
		fmt.Fprintln(stderr, "  yarmouth cache -r <raiz> list                 muestra la cache")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	sub := fs.Arg(0)
	rest := fs.Args()[1:]
	switch sub {
	case "export", "exportar":
		return runCacheExport(*root, rest, stdout, stderr)
	case "import", "importar":
		return runCacheImport(*root, rest, stdout, stderr)
	case "list", "lista":
		return runCacheList(*root, stdout, stderr)
	default:
		fs.Usage()
		return 2
	}
}

func runCacheExport(root string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "directorio destino (se crea si no existe)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(stderr, "yarmouth cache export: falta -out <dir>")
		return 2
	}
	matches, err := repo.CachedPackages(root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth cache export: %v\n", err)
		return 1
	}
	if len(matches) == 0 {
		fmt.Fprintln(stderr, "yarmouth cache export: la cache esta vacia (corre yarmouth sync primero)")
		return 1
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(stderr, "yarmouth cache export: %v\n", err)
		return 1
	}
	for _, m := range matches {
		src := filepath.Join(repo.CacheRoot(root), m.Pkg.Filename())
		dst := filepath.Join(*out, m.Pkg.Filename())
		if err := copyTo(src, dst); err != nil {
			fmt.Fprintf(stderr, "yarmouth cache export: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "  %-8s %s sha256 %s\n", m.Remote.Alias, m.Pkg.Filename(), m.Pkg.SHA256)
	}
	fmt.Fprintf(stdout, "yarmouth cache export: %d .yrm exportados (certificados por indice firmado)\n", len(matches))
	return 0
}

func runCacheImport(root string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "directorio con los .yrm a importar")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" {
		fmt.Fprintln(stderr, "yarmouth cache import: falta -dir <dir>")
		return 2
	}
	entries, err := os.ReadDir(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth cache import: %v\n", err)
		return 1
	}
	var okN, rej int
	for _, e := range entries {
		if e.IsDir() || !stringsHasSuffix(e.Name(), ".yrm") {
			continue
		}
		src := filepath.Join(*dir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth cache import: %v\n", err)
			return 1
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		m, cert, err := repo.LocalMatchBySHA(root, sha)
		if err != nil {
			fmt.Fprintf(stderr, "yarmouth cache import: %v\n", err)
			return 1
		}
		if !cert {
			fmt.Fprintf(stderr, "yarmouth cache import: rechazado %s (sha256 %s no certificado por ningun indice firmado local; corre yarmouth sync/verify)\n", e.Name(), sha[:16])
			rej++
			continue
		}
		if m.Pkg.Filename() != e.Name() {
			fmt.Fprintf(stderr, "yarmouth cache import: rechazado %s (el indice firma el paquete %s con otro nombre)\n", e.Name(), m.Pkg.Filename())
			rej++
			continue
		}
		dst := filepath.Join(repo.CacheRoot(root), m.Pkg.Filename())
		if err := copyTo(src, dst); err != nil {
			fmt.Fprintf(stderr, "yarmouth cache import: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "  importado %s desde %s (certificado %s)\n", m.Pkg.Filename(), m.Remote.Alias, m.Pkg.SHA256)
		okN++
	}
	if rej > 0 {
		fmt.Fprintf(stderr, "yarmouth cache import: %d rechazados (huerfanos o con nombre distinto al certificado)\n", rej)
	}
	fmt.Fprintf(stdout, "yarmouth cache import: %d .yrm importados (todos verificados por sha256 contra indice firmado)\n", okN)
	return 0
}

func runCacheList(root string, stdout, stderr io.Writer) int {
	matches, err := repo.CachedPackages(root)
	if err != nil {
		fmt.Fprintf(stderr, "yarmouth cache list: %v\n", err)
		return 1
	}
	if len(matches) == 0 {
		fmt.Fprintln(stdout, "yarmouth cache list: la cache esta vacia (corre yarmouth sync para llenarla)")
		return 0
	}
	for _, m := range matches {
		fmt.Fprintf(stdout, "  %-8s %s\t%s\t(%d bytes)\n", m.Remote.Alias, m.Pkg.Filename(), m.Pkg.SHA256, m.Pkg.Size)
	}
	fmt.Fprintf(stdout, "yarmouth cache list: %d .yrm en la cache, todos con sha256 verificado contra indice firmado\n", len(matches))
	return 0
}

// copyTo copia un archivo sin resolucion de enlaces: se usa para mover .yrm
// entre la cache local y un directorio externo, manteniendo siempre la
// verificacion por sha256 que ya ocurrio antes de llamar a esta funcion.
func copyTo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func init() {
	commands = append(commands, command{
		name:    "cache",
		short:   "exporta/importa/lista la cache local de .yrm",
		usage:   "yarmouth cache -r <raiz> <export|import|list> ...",
		details: "opera sobre la cache local (raiz/var/cache/yarmouth) con la garantia auditable de sha256: export copia fuera SOLO .yrm certificados (sha256 verificado contra indice firmado) a un -out <dir>; import acepta en la cache SOLO .yrm cuyo sha256 coincida con un indice firmado sincronizado (un .yrm huerfano/manipulado se rechaza y nunca entra: seguro+agil+auditable); list muestra la cache con alias, sha256 y tamano. Esto permite mover .yrm por USB/LFS sin red y sin perdida de verificacion.",
		run:     runCacheCmd,
	})
}
