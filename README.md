# Yarmouth

Gestor de paquetes para sistemas Linux From Scratch (LFS), escrito en **Go**.

Yarmouth usa un modelo híbrido: paquetes binarios `.yrm` (tar.xz con manifiesto y
verificación criptográfica) servidos desde repositorios remotos, y (en desarrollo)
recetas de construcción desde el código fuente. Funciona tanto dentro de un chroot
en construcción como en un sistema arrancado.

Inspirado en `dnf`, `xbps` y `pkg`; diseñado para un solo usuario, sin `systemd`,
sencillo y auditable.

## Estado

| Fase | Contenido | Estado |
|------|-----------|--------|
| 0 | Formato `.yrm`, manifiesto, índice `repodata`, `build` / `index` | ✅ |
| 1 | Instalación/eliminación, base de datos `pkgdb.json`, hooks, `-r` (chroot) | ✅ |
| 2 | Repositorios remotos, `sync`, instalación por nombre con verificación sha256 | ✅ |
| 3 | Resolución de dependencias, `upgrade`, `world` | ⏳ siguiente |
| 4 | Firma/verificación de paquetes, `query` | pendiente |
| 5 | Recetas de construcción (bash → YAML) | pendiente |

Versión actual: **0.2.0**.

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

# Instalar por nombre (resuelve desde los repositorios)
yarmouth install -r /mnt/lfs hello

# También se admite un .yrm local
yarmouth install -r /mnt/lfs ./hello-0.1.0-1.x86_64.yrm

yarmouth list -r /mnt/lfs          # paquetes instalados
yarmouth remove -r /mnt/lfs hello  # desinstala (con hooks y limpieza)
yarmouth remove -f -r /mnt/lfs x   # fuerza sobre dependientes
```

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
hardlinks propietarios, hooks y si se instaló manualmente. Se escribe de forma
atómica (archivo temporal + rename). `remove` borra solo lo que es propiedad del
paquete, en orden profundo primero.

## Estructura del código

```
cmd/yarmouth/        CLI: build, index, repo, sync, install, remove, list
internal/archive/    formato .yrm (crear/abrir/verificar/extraer, reproducible)
internal/metadata/   manifiesto INI estricto
internal/version/    comparación de versiones estilo Debian
internal/repo/       índice repodata y repositorios remotos (sync/resolver/descargar)
internal/db/         inventario pkgdb.json
internal/install/    instalación/eliminación con hooks y rollback
examples/hellopkg/   paquete de ejemplo con hooks de instalación/eliminación
```

## Roadmap

- **Fase 3** — resolución real de dependencias (topológica, transactiva),
  `upgrade` y marcas `world`/`auto`.
- **Fase 4** — firmas (p. ej. ed25519), verificación al instalar y `query`.
- **Fase 5** — recetas de construcción del paquete desde el código fuente.

## Licencia

GPL-3.0-only. Véase `LICENSE`.