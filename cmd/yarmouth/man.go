package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// man se registra con init() y no en el literal de commands: su cuerpo
// iteraria la propia tabla, y en Go tomar el valor de una funcion que usa un
// paquete-nivel global arrastra ese global al grafo de inicializacion (ciclo
// commands -> runMan -> commands). Con init() el cuerpo queda fuera del grafo
// y man sigue apareciendo en -h, help y man(1) con la misma fuente unica.

func init() {
	commands = append(commands, command{
		name:    "man",
		short:   "genera las paginas de manual en roff",
		usage:   "yarmouth man [1|5]",
		details: "genera las paginas de manual a partir de la misma tabla que alimenta -h y help: resumen, sintaxis y detalles de cada comando nunca pueden desincronizarse. yarmouth man 1 produce yarmouth.1 (todos los comandos); yarmouth man 5 produce yarmouth.conf.5 (formatos de repos.conf, trusted-keys, repodata, .yrm, recetas .yarmouth y pkgdb.json). Para instalar: yarmouth man 1 > /usr/share/man/man1/yarmouth.1 y yarmouth man 5 > /usr/share/man/man5/yarmouth.conf.5.",
		run:     runMan,
	})
}

// man produce las paginas de manual en roff a partir de la misma tabla que
// alimenta -h y help: el resumen, la sintaxis y los detalles de cada comando
// nunca pueden desincronizarse de la pagina. Seccion 1 renderiza yarmouth.1;
// seccion 5 renderiza yarmouth.conf.5 (formatos de configuracion y datos).

const manReport = "septiembre 2026"

// init re-registra el comando "man": el cuerpo de writeManX itera la tabla
// commands, asi que si "man" estuviera en el literal de esa tabla el grafo de
// inicializacion seria ciclico (commands -> runMan -> writeMan1 -> commands)
// y el compilador lo rechaza. Con init() el registro ocurre en tiempo de
// ejecucion, rompiendo el ciclo, y man sigue apareciendo en -h, help y help
// man con la misma informacion que el resto.
func init() {
	commands = append(commands, command{
		name:    "man",
		short:   "genera las paginas de manual en roff",
		usage:   "yarmouth man [1|5]",
		details: "imprime en roff la pagina de manual completa a partir de la misma tabla que alimenta -h y help: el resumen, la sintaxis y los detalles de cada comando nunca pueden desincronizarse. Seccion 1 genera yarmouth.1 (todos los comandos y opciones); seccion 5 genera yarmouth.conf.5 (formatos de repos.conf, trusted-keys, repodata, el paquete .yrm, pkgdb.json). Para instalar: yarmouth man 1 > /usr/share/man/man1/yarmouth.1 y yarmouth man 5 > /usr/share/man/man5/yarmouth.conf.5. Tambien lo usa make man.",
		run:     runMan,
	})
}

func runMan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("man", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "genera las paginas de manual en roff a partir de la ayuda integrada")
		fmt.Fprintln(stderr, "\nUso: yarmouth man [1|5]")
		fmt.Fprintln(stderr, "  1    yarmouth.1  — todos los comandos y opciones (sec. 1)")
		fmt.Fprintln(stderr, "  5    yarmouth.conf.5 — formatos de configuracion (sec. 5)")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "yarmouth man: solo se admite un numero de seccion")
		return 2
	}
	section := "1"
	if fs.NArg() == 1 {
		section = fs.Arg(0)
	}
	switch section {
	case "1":
		writeMan1(stdout)
	case "5":
		writeMan5(stdout)
	default:
		fmt.Fprintf(stderr, "yarmouth man: seccion %q no disponible (1 o 5)\n", section)
		return 2
	}
	return 0
}

// troff escapa un texto para roff y mantiene la codificacion ISO-8859-1.
func troff(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\e`)
		case '-':
			b.WriteString(`\-`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func manHeader(w io.Writer, title, section, name string) {
	fmt.Fprintf(w, ".TH %s %s %q \"yarmouth %s\" \"Manual de yarmouth\"\n", title, section, manReport, version)
	fmt.Fprintf(w, ".SH NOMBRE\n%s\n", name)
}

func writeMan1(w io.Writer) {
	manHeader(w, "YARMOUTH", "1", "yarmouth \\- gestion de paquetes para sistemas LFS")
	fmt.Fprintf(w, ".SH SINOPSIS\n.B yarmouth\n\\fIcomando\\fR [\\fIopciones\\fR]\n.PP\n.B yarmouth help [\\fIcomando\\fR]\n.PP\n.B yarmouth\\-V\\fR\\fI\\-V\\fR  |  .B \\-\\-version\n")
	fmt.Fprintf(w, ".SH DESCRIPCION\nYarmouth es un gestor de paquetes binarios \\fI.yrm\\fR para sistemas Linux From Scratch, inspirado en dnf, xbps y pkg.\n.PP\nGestona repositorios remotos con indice \\fIrepodata\\fR firmado (ed25519), resuelve dependencias deps-first, ejecuta hooks de instalacion/eliminacion con rollback, y tiene un modo de construccion reproducible por recetas \\fI.yarmouth\\fR.\n.PP\nLos comandos que escriben (\\fBinstall\\fR, \\fBremove\\fR, \\fBupgrade\\fR, \\fBrepo\\fR, \\fBkey\\fR, \\fBsync\\fR) operan sobre la base citada con \\fI\\-r\\fR (por defecto \\fI/\\fR); en una maquina LFS arrancada suelen necesitar privilegios (doas). Los de consulta (\\fBquery\\fR, \\fBcheck\\fR, \\fBlist\\fR) son de solo lectura.\n")
	fmt.Fprintf(w, ".SH COMANDOS\n")
	for _, c := range commands {
		fmt.Fprintf(w, ".SS %s\n%s\n.PP\n\\fIUso:\\fR %s\n", troff(c.name), troff(c.short), troff(c.usage))
		if c.details != "" {
			fmt.Fprintf(w, ".PP\n%s\n", troff(c.details))
		}
	}
	fmt.Fprintf(w, ".SH OPCIONES GLOBALES\n.TP\n.BI \\-h \" \" \\-\\-help\nmuestra la ayuda\n.TP\n.BI \\-V \" \" \\-\\-version\nmuestra la version\n")
	fmt.Fprintf(w, ".SH ARCHIVOS\n.TP\n.B \\fI<raiz>/etc/yarmouth/repos.conf\\fR\nconfiguracion de repositorios (ver yarmouth.conf(5))\n.TP\n.B \\fI<raiz>/etc/yarmouth/trusted\\-keys\\fR\nclaves publicas de confianza\n.TP\n.B \\fI<raiz>/var/db/yarmouth/pkgdb.json\\fR\nbase de datos de paquetes instalados\n")
	fmt.Fprintf(w, ".SH VER TAMBIEN\n.BR yarmouth.conf (5)\n")
}

func writeMan5(w io.Writer) {
	manHeader(w, "YARMOUTH.CONF", "5", "yarmouth.conf \\- formatos de configuracion y datos de yarmouth")
	fmt.Fprintf(w, ".SH DESCRIPCION\nEsta pagina documenta los archivos que yarmouth lee y escribe bajo la base \\fI<raiz>\\fR (por defecto \\fI/\\fR) sobre la que opera con \\fI\\-r\\fR.\n")
	fmt.Fprintf(w, ".SH REPOS\\&.CONF\n.B \\fI<raiz>/etc/yarmouth/repos.conf\\fR\n.PP\nUna linea por repositorio remoto, campos separados por tabulador:\n.PP\n\\fIalias\\tURL\\t[clave-publica-ed25519]\\fR\n.PP\nLa clave es opcional y se configura con \\fByarmouth repo add \\-k\\fR. Sin ella, el indice se acepta sin firmar (para desarrollo). Con ella, \\fBsync\\fR exige \\fIrepodata.sig\\fR verificable por esa clave antes de poblar la cache.\n")
	fmt.Fprintf(w, ".SH TRUSTED\\&.KEYS\n.B \\fI<raiz>/etc/yarmouth/trusted\\-keys\\fR\n.PP\nUna linea por clave publica ed25519 (hex, 64 caracteres), admitiendo comentarios con \\fI#\\fR. Se administra con \\fByarmouth key add/del/list\\fR. Si el archivo contiene al menos una clave, \\fBinstall\\fR rechaza cualquier paquete sin firmar o no verificable por esas claves (y \\fBupgrade\\fR pasa a exigir paquetes firmados). Con el archivo vacio (modo desarrollo) se acepta cualquier paquete.\n")
	fmt.Fprintf(w, ".SH REpODATA\n.B \\fIrepodata\\fR\n.PP\nIndice de un repositorio: lista de \\fIarchivo\\tyrm\\tTAMANO\\tsha256\\fR, una por paquete, generado con \\fByarmouth index\\fR. La firma es un ed25519 de su contenido en \\fIrepodata.sig\\fR (\\fByarmouth sign\\fR).\n")
	fmt.Fprintf(w, ".SH PAQUETE .YRM\n.PP\nUn \\fI.yrm\\fR es un tar.xz con estructura interna documentada en \\fByarmouth build\\fR y en el README: manifiesto INI estricto, contenido con uid/gid 0 y mtime 0 (construccion reproducible), hooks opcionales \\fIyarmouth/*\\fR y firma ed25519 embebida (\\fByarmouth sign\\fR).\n")
	fmt.Fprintf(w, ".SH PKGDB\\&.JSON\n.B \\fI<raiz>/var/db/yarmouth/pkgdb.json\\fR\n.PP\nBase de datos de lo instalado: paquete \\-> lista de archivos con su sha256, dependencias (auto/manual) y hooks. Se mantiene con \\fBinstall/remove/upgrade\\fR y se verifica con \\fByarmouth check\\fR.\n")
	fmt.Fprintf(w, ".SH VER TAMBIEN\n.BR yarmouth (1)\n")
}
