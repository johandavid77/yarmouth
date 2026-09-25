package main

import (
	"fmt"
	"io"
	"os"
)

const version = "0.7.0"

type command struct {
	name    string
	short   string
	usage   string
	details string
	run     func(args []string, stdout, stderr io.Writer) int
}

var commands = []command{
	{
		name:    "build",
		short:   "construye un paquete .yrm desde un DESTDIR o una receta",
		usage:   "yarmouth build -manifest <manifest> -destdir <dir> [opciones]",
		details: "construye el paquete .yrm que se publica en un repositorio. En modo DESTDIR empaqueta el contenido de -destdir descrito por -manifest (INI estricto: pkgname, pkgver, buildid, arch, license, depends) e incorpora los hooks opcionales. En modo receta (-recipe) lee un .yarmouth: descarga las fuentes y verifica su sha256, ejecuta prepare/build/install con bash -e y escribe el .yrm en -out. La salida es reproducible.",
		run:     runBuild,
	},
	{
		name:    "index",
		short:   "crea el indice repodata de un directorio",
		usage:   "yarmouth index -dir <dir> -out <repodata>",
		details: "escanea -dir en busca de paquetes .yrm y genera el indice repodata -out. Ese indice es el que npm publica como repositorio; luego se firma con sign -k (y se publica repodata.sig).",
		run:     runIndex,
	},
	{
		name:    "repo",
		short:   "administra repositorios (add/del/list)",
		usage:   "yarmouth repo -r <raiz> <add|del|list> ...",
		details: "administra los repositorios remotos en repos.conf de la raiz (-r). add registra un alias con su URL y, opcionalmente con -k, la clave publica ed25519 que debe firmar el indice; del lo elimina; list los muestra. sync usa esta configuracion.",
		run:     runRepoCmd,
	},
	{
		name:    "sync",
		short:   "descarga los indices de los repositorios",
		usage:   "yarmouth sync -r <raiz>",
		details: "descarga el repodata de cada repositorio configurado (repo add) a la cache de la raiz (-r). Si el repo tiene clave configurada, exige repodata.sig verificable por ella (y los paquetes no firmados seran rechazados al instalar); sin clave, acepta el indice sin firmar (solo para desarrollo local).",
		run:     runSync,
	},
	{
		name:    "install",
		short:   "instala un paquete (por nombre o .yrm)",
		usage:   "yarmouth install -r <raiz> <paquete|paquete.yrm>",
		details: "instala un paquete por nombre (resolviendo la cadena de dependencias desde los indices ya sincronizados) o un .yrm local. Si hay claves de confianza configuradas (key add), todo paquete debe estar firmado y verificable por ellas (ademas de hooks opcionales sin firmar que el instalador ejecuta); sin claves, acepta cualquier paquete (modo desarrollo).",
		run:     runInstall,
	},
	{
		name:    "remove",
		short:   "desinstala un paquete",
		usage:   "yarmouth remove -r <raiz> [-o] <paquete>",
		details: "desinstala un paquete ejecutando sus hooks de eliminacion. -o elimina tambien las dependencias auto que quedan huerfanas; -f fuerza la baja aunque haya dependientes.",
		run:     runRemove,
	},
	{
		name:    "upgrade",
		short:   "actualiza los paquetes instalados",
		usage:   "yarmouth upgrade -r <raiz> [paquete...]",
		details: "actualiza a la version mas reciente los paquetes indicados (o todos los instalados) resolviendo el plan completo (dependencias nuevas incluidas, deps-first). -l solo lista las actualizaciones pendientes sin aplicar nada: no descarga, no escribe, no requiere permisos. Message: 'sistema actualizado' cuando no hay nada mas reciente.",
		run:     runUpgrade,
	},
	{
		name:    "keygen",
		short:   "genera un par de claves ed25519",
		usage:   "yarmouth keygen -out <archivo>",
		details: "genera un par de claves ed25519 y escribe la privada (hex) en -out. La clave publica se obtiene con keygen -pub o pubkey; es la que se distribuye para repo add -k y key add.",
		run:     runKeygen,
	},
	{
		name:    "key",
		short:   "administra claves de confianza (add/del/list)",
		usage:   "yarmouth key -r <raiz> <add|del|list> ...",
		details: "administra la lista de claves publicas de confianza en etc/yarmouth/trusted-keys de la raiz. Con al menos una clave, todo paquete a instalar debe estar firmado y poder verificarse con alguna de ellas; sin claves, yarmouth funciona en modo desarrollo y acepta paquetes sin firmar.",
		run:     runKeyCmd,
	},
	{
		name:    "sign",
		short:   "firma un .yrm o un repodata",
		usage:   "yarmouth sign -k <clave> <paquete.yrm|repodata>",
		details: "firma con ed25519 un paquete .yrm (la firma se embebe en el archivo) o un indice repodata (escribe repodata.sig al lado). Los paquetes/.yrm se verifican al instalar cuando hay claves de confianza; la firma del repodata la exige sync si el repo tiene clave.",
		run:     runSign,
	},
	{
		name:    "query",
		short:   "detalles de un paquete instalado",
		usage:   "yarmouth query -r <raiz> [-f] <paquete>",
		details: "muestra el detalle de un paquete instalado: version completa, arch, hash del contenido, dependencias y, con -f, la lista de archivos que instalo.",
		run:     runQuery,
	},
	{
		name:    "check",
		short:   "verifica la integridad de lo instalado",
		usage:   "yarmouth check -r <raiz>",
		details: "recalcula el sha256 de cada archivo instalado y lo compara con el hash registrado en la base de datos (pkgsdb). Detecta archivos modificados, borrados o anadidos; devuelve codigo 1 si hay algun problema.",
		run:     runCheck,
	},
	{
		name:    "list",
		short:   "lista los paquetes instalados",
		usage:   "yarmouth list -r <raiz> [patron]",
		details: "lista los paquetes instalados, mostrando su version completa y si fueron instalados manualmente o como dependencia automatica. Con un patron, filtra por nombre.",
		run:     runList,
	},
	{
		name:    "help",
		short:   "muestra ayuda (yarmouth help <comando>)",
		usage:   "yarmouth help <comando>",
		details: "muestra la ayuda general o, con un argumento, la de un comando concreto (misma informacion que aparece en la pagina de manual yarmouth(1), generada con yarmouth man 1).",
	},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) > 1 {
			printCmdHelp(stderr, args[1])
		} else {
			printUsage(stderr)
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
	printUsage(stderr)
	return 2
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, "yarmouth %s — gestion de paquetes para sistemas LFS\n\nUso:\n  yarmouth <comando> [opciones]\n\nComandos:\n", version)
	for _, c := range commands {
		fmt.Fprintf(w, "  %-9s %s\n", c.name, c.short)
	}
	fmt.Fprintf(w, "  %-9s %s\n", "help", "muestra ayuda (yarmouth help <comando>)")
	fmt.Fprintf(w, "  %-9s %s\n\nOpciones globales:\n  -h, --help    muestra esta ayuda\n  -V, --version muestra la version\n", "help", "ayuda general")
}

func printCmdHelp(w io.Writer, name string) {
	for _, c := range commands {
		if c.name == name {
			fmt.Fprintf(w, "%s — %s\n\n%s\n\nUso:\n  %s\n", c.name, c.short, c.details, c.usage)
			return
		}
	}
	fmt.Fprintf(w, "yarmouth: no existe el comando %q\n", name)
}
