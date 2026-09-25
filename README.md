# Yarmouth

Gestor de paquetes para sistemas Linux From Scratch (LFS), escrito en **Go**.

Yarmouth usa un modelo híbrido: paquetes binarios `.yrm` (tar.xz con manifiesto y
verificación criptográfica) servidos desde repositorios remotos, y recetas
`.yarmouth` que construyen esos paquetes desde el código fuente. Funciona tanto
dentro de un chroot en construcción como en un sistema arrancado.

Inspirado en `dnf`, `xbps` y `pkg`; diseñado para un solo usuario, sin `systemd`,
sencillo y auditable.

> El nombre honra al SS _Yarmouth_, el primer barco de la **Black Star Line** de
> Marcus Garvey: lo que "embarca" los paquetes y los "descarga" en tu LFS.

## Estado

| Fase | Contenido | Estado |
|------|-----------|--------|
| 0 | Formato `.yrm`, manifiesto, índice `repodata`, `build` / `index` | ✅ |
| 1 | Instalación/eliminación, base de datos `pkgdb.json`, hooks, `-r` (chroot) | ✅ |
| 2 | Repositorios remotos, `sync`, instalación por nombre con verificación sha256 | ✅ |
| 3 | Resolución de dependencias, `upgrade`, `world` / `auto` | ✅ |
| 4 | Firmas ed25519, verificación al instalar, `query` / `check` | ✅ |
| 5 | Recetas de construcción (`.yarmouth`, bash → YAML) | ✅ |

Versión actual: **0.5.0**.

## Requisitos

- Go ≥ 1.22 (probado con Go 1.27).
- Fuera de LFS: solo herramientas GNU estándar (`tar`, `xz`, `sh`).

## Compilar

```sh
make            # -> bin/yarmouth
make test       # go test ./...
make vet        # go vet ./...
make clean
```

Los tests no tocan el sistema: usan directorios temporales.

## Uso rápido

```sh
# Repositorio local
yarmouth index -dir /srv/repo -out /srv/repo/repodata

# Configuración (un repositorio por alias)
yarmouth repo -r /mnt/lfs add myrepo https://raw.githubusercontent.com/USUARIO/yarmouth/main/repo
yarmouth sync -r /mnt/lfs

# Instalar por nombre (resuelve la cadena de dependencias; las libs van "auto")
yarmouth install -r /mnt/lfs app

# Actualiza a la version mas reciente (paquete concreto o todos)
yarmouth upgrade -r /mnt/lfs
yarmouth upgrade -r /mnt/lfs app

yarmouth list -r /mnt/lfs            # paquetes instalados (manual/auto)
yarmouth remove -r /mnt/lfs app      # desinstala (con hooks y limpieza)
yarmouth remove -o -r /mnt/lfs app   # ademas elimina las dependencias huerfanas
yarmouth remove -f -r /mnt/lfs x     # fuerza sobre dependientes

# También se admite un .yrm local (resuelve sus dependencias de los repos)
yarmouth install -r /mnt/lfs ./app-1.0-1.x86_64.yrm

yarmouth query -r /mnt/lfs app     # detalles de un paquete instalado
yarmouth check -r /mnt/lfs         # verifica la integridad de lo instalado
```

## Recetas de construcción (`.yarmouth`)

Una receta declara las fuentes (con su sha256) y los pasos bash para preparar,
compilar e instalar:

```sh
pkgname = app
pkgver = 1.0
buildid = 1
arch = x86_64
description = una aplicacion cualquiera
license = MIT
homepage = https://...
sources = https://.../app-1.0.tar.gz
sha256 = 32caracteres-hex-por-fuente...
prepare = autoreconf -i            # opcional, en el srcdir
build = ./configure --prefix=/usr  # cada linea es una instruccion
build = make
install = make install DESTDIR=$DESTDIR
post-install = echo 'app lista'   # hooks opcionales (pre-install, ...)
```

Construcción (descarga, verifica sha256, ejecuta los pasos y produce el `.yrm`):

```sh
yarmouth build -recipe pkg.yarmouth -out repo
```

- Cada fuente requiere un `sha256`; el descenso falla si no coincide.
- Soporta archivos locales/URLs y tarballs (`.tar`, `.gz`, `.xz`, `.bz2`,
  `.zip`); si el tarball deja un único directorio, ese es el `srcdir`.
- Variables disponibles en los pasos: `SRCDIR`, `SRCROOT`, `DESTDIR`, `PREFIX`
  (=/usr), `PKGNAME`, `PKGVER`, `BUILDID`, `ARCH`. Los pasos corren con bash
  (`set -e`) en `$SRCDIR`, el `install` escribe en `$DESTDIR`.
- Las rutas de fuente relativas se resuelven respecto a la receta; los hashes y
  la salida son reproducibles (uid/gid=0, mtime=0).

## Firmas ed25519

Flujo del publicador:

```sh
yarmouth keygen -out mantenedor.key          # guarda la privada, imprime la publica
yarmouth pubkey mantenedor.key               # re-imprime la publica cuando se precise
yarmouth sign -k mantenedor.key app-1.0-1.x86_64.yrm   # firma embebida en el .yrm
yarmouth index -dir repo -out repo/repodata
yarmouth sign -k mantenedor.key repo/repodata          # crea repodata.sig
```

El cliente declara la confianza por repositorio:

```sh
yarmouth key -r /mnt/lfs add <PUB>
yarmouth repo -r /mnt/lfs add -k <PUB> main https://...
yarmouth sync -r /mnt/lfs   # exige y verifica repodata.sig
```

- La firma de paquete es un **ed25519 sobre la cadena canónica**
  (pkgname + pkgver + buildid + arch + datahash) embebida como entrada de
  control `yarmouth/signature`, fuera del `datahash`.
- Si hay claves de confianza en `<raiz>/etc/yarmouth/trusted-keys`, `install`
  **rechaza** cualquier paquete sin firmar o no verificable por esas claves
  (`keygen` imprime la pública; `key add/del/list` administra el keyring).
- `sync` solo popula la caché si el `repodata.sig` verifica contra la clave
  del repositorio; un tiempo de espera con servidor comprometido se detecta.
- `yarmouth check -r <raiz>` verifica los sha256 de todos los archivos
  instalados contra el inventario (detecta manipulación en disco).

`-r <raiz>` indica el directorio raíz (chroot). Por defecto es `/`. Las
instalaciones jamás tocan la máquina de desarrollo: diríjanse al root de LFS.

## Formato de paquete `.yrm`

Un `.yrm` es un tarball con compresión xz con esta estructura:

```
yarmouth/manifest            Manifiesto INI (véase abajo)
yarmouth/pre-install         Hook ejecutado antes de copiar archivos (opcional)
yarmouth/post-install        Hook ejecutado después (opcional)
yarmouth/pre-remove          Hook antes de eliminar (opcional)
yarmouth/post-remove         Hook después de eliminar (opcional)
usr/bin/hello                Contenido del paquete
usr/share/doc/hello/README
```

- Los archivos de contenido se escriben con `uid/gid=0` y `mtime=0` → **builds
  bit a bit reproducibles** (mismo `.yrm` para el mismo manifest + DESTDIR).
- Soporta directorios, symlinks y hardlinks.
- Recibe `yarmouth build` desde un `DESTDIR`:

```sh
yarmouth build -manifest examples/hellopkg/manifest -destdir examples/hellopkg/destdir -out hello-0.1.0-1.x86_64.yrm
```

### Manifiesto

Formato INI estricto (clave = valor, una por línea). Campos:

| Campo | Descripción |
|-------|-------------|
| `pkgname` | nombre del paquete |
| `pkgver` | versión (semver/Debian) |
| `buildid` | revisión de empaquetado |
| `arch` | arquitectura (x86_64, aarch64, i686, noarch) |
| `description` | descripción |
| `license` | licencia del software empacado |
| `homepage` | página del proyecto |
| `maintainer` | responsable del paquete |
| `depends` | dependencias, separadas por coma |

Durante `index` / `install` el paquete se enriquece con: `sha256` de cada
archivo, `datahash` (sha256 de todo el contenido no-control) y `sha256` del
archivo `.yrm` completo. Los hooks se ejecutan con `sh` en la raíz de destino y
estas variables de entorno:

- `YK_ROOT` — raíz de instalación
- `YK_PKGNAME`, `YK_PKGVERSION` — paquete en curso

## Índice de repositorio (`repodata`)

Línea de cabecera `# yarmouth repodata v1`, luego un paquete por línea,
separado por tabuladores:

```
nombre<TAB>ver-arch-de-build<TAB>arch<TAB>sha256<TAB>tamano<TAB>dependencias(,)
```

`yarmouth sync` descarga este índice (validando su sha256) y lo guarda en
`<raiz>/var/cache/yarmouth/<alias>.repodata`. `install` por nombre lo consulta,
elige la **versión más reciente** de entre todos los repositorios, descarga el
`.yrm` a `<raiz>/var/cache/yarmouth/` y verifica **tamaño + sha256** antes de
instalar.

## Base de datos local

`<raiz>/var/db/yarmouth/pkgdb.json` conserva el inventario de paquetes
instalados: versión, dependencias declaradas, archivos/directorios/symlinks/
hardlinks propietarios, hooks y si se instaló **manual (world)** o **auto**
(aunque sea dependencia). Se escribe de forma atómica (archivo temporal +
rename). `remove` borra solo lo que es propiedad del paquete, en orden profundo
primero, y con `-o` poda las dependencias `auto` que quedan huérfanas.
`install` resuelve la cadena de dependencias en orden deps-first: las
dependencias ya instaladas se consideran satisfechas y las nuevas se marcan
`auto`. `upgrade` sustituye cada paquete por la versión más reciente disponible,
preservando la marca manual/auto, eliminando archivos obsoletos y agregando
como `auto` las dependencias nuevas.

## Estructura del código

```
cmd/yarmouth/        CLI: build, index, repo, sync, install, remove, list, upgrade,
                     keygen, key, pubkey, sign, query, check
internal/archive/    formato .yrm (crear/abrir/verificar/extraer, reproducible)
internal/metadata/   manifiesto INI estricto
internal/version/    comparación de versiones estilo Debian
internal/recipe/     recetas .yarmouth: fuentes, sha256 y pasos de construcción
internal/repo/       índice repodata y repositorios remotos (sync/resolver/descargar)
internal/resolve/    plan de instalación: dependencias, ciclos, upgrade
internal/sig/        claves ed25519 y keyring de confianza (trusted-keys)
internal/db/         inventario pkgdb.json
internal/install/    instalación/eliminación con hooks, rollback y autoremove
examples/hellopkg/   paquete construido con manifest + DESTDIR
examples/receta/     paquete construido desde una receta .yarmouth
```

## Roadmap

- **Fase 6** — recetas declarativas (YAML), caché de fuentes y construcción en chroot.

## Licencia

GPL-3.0-only. Véase `LICENSE`.