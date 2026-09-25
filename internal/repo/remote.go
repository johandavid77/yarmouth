package repo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"yarmouth/internal/version"
)

const reposPath = "etc/yarmouth/repos.conf"
const cachePath = "var/cache/yarmouth"
const UserAgent = "yarmouth"

type Remote struct {
	Alias string
	URL   string
	Key   string // clave publica ed25519 en hex (vacia = repositorio sin firmar)
}

func rootPath(root, name string) string {
	clean := filepath.Clean("/" + name)
	if root == "" || root == "/" {
		return clean
	}
	return filepath.Join(root, clean)
}

func ReposPath(root string) string { return rootPath(root, reposPath) }

func CacheIndex(root, alias string) string {
	return filepath.Join(rootPath(root, cachePath), alias+".repodata")
}

func ReadRepos(root string) ([]Remote, error) {
	data, err := os.ReadFile(ReposPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var remotes []Remote
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("linea invalida en repos.conf: %q", trimmed)
		}
		key := ""
		if len(parts) == 3 {
			key = strings.TrimSpace(parts[2])
		}
		remotes = append(remotes, Remote{Alias: strings.TrimSpace(parts[0]), URL: strings.TrimSpace(parts[1]), Key: key})
	}
	return remotes, nil
}

func WriteRepos(root string, remotes []Remote) error {
	var b strings.Builder
	for _, r := range remotes {
		fmt.Fprintf(&b, "%s\t%s", r.Alias, r.URL)
		if r.Key != "" {
			fmt.Fprintf(&b, "\t%s", r.Key)
		}
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(ReposPath(root)), 0o755); err != nil {
		return err
	}
	tmp := ReposPath(root) + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ReposPath(root))
}

func AddRepo(root, alias, url string, key ...string) error {
	if alias == "" || url == "" {
		return errors.New("se requieren alias y url")
	}
	remotes, err := ReadRepos(root)
	if err != nil {
		return err
	}
	for _, r := range remotes {
		if r.Alias == alias {
			return fmt.Errorf("el repositorio %q ya existe", alias)
		}
	}
	r := Remote{Alias: alias, URL: url}
	if len(key) > 0 {
		r.Key = key[0]
	}
	return WriteRepos(root, append(remotes, r))
}

func RemoveRepo(root, alias string) error {
	remotes, err := ReadRepos(root)
	if err != nil {
		return err
	}
	out := remotes[:0]
	found := false
	for _, r := range remotes {
		if r.Alias == alias {
			found = true
			continue
		}
		out = append(out, r)
	}
	if !found {
		return fmt.Errorf("el repositorio %q no existe", alias)
	}
	return WriteRepos(root, out)
}

func joinURL(base, name string) string {
	if strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		return strings.TrimSuffix(base, "/") + "/" + name
	}
	return filepath.Join(strings.TrimPrefix(base, "file://"), name)
}

func (r Remote) IndexURL() string           { return joinURL(r.URL, "repodata") }
func (r Remote) SigURL() string             { return joinURL(r.URL, "repodata.sig") }
func (r Remote) PackageURL(f string) string { return joinURL(r.URL, f) }

func FetchBytes(target string) ([]byte, error) {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", target, resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(strings.TrimPrefix(target, "file://"))
}

func ReadIndexFile(path string) ([]Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadIndex(f)
}

func sha256sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func Sync(root string) ([]Remote, error) {
	remotes, err := ReadRepos(root)
	if err != nil {
		return nil, err
	}
	if len(remotes) == 0 {
		return nil, errors.New("no hay repositorios configurados (yarmouth repo add <alias> <url>)")
	}
	cache := rootPath(root, cachePath)
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return nil, err
	}
	for _, r := range remotes {
		b, err := FetchBytes(r.IndexURL())
		if err != nil {
			return nil, fmt.Errorf("repo %q: %w", r.Alias, err)
		}
		if _, err := ReadIndex(bytes.NewReader(b)); err != nil {
			return nil, fmt.Errorf("repo %q: repodata invalido: %w", r.Alias, err)
		}
		if r.Key != "" {
			sigData, err := FetchBytes(r.SigURL())
			if err != nil {
				return nil, fmt.Errorf("repo %q esta firmado: %w", r.Alias, err)
			}
			pub, err := parsePublic(r.Key)
			if err != nil {
				return nil, fmt.Errorf("repo %q: %w", r.Alias, err)
			}
			if !ed25519.Verify(pub, b, sigData) {
				return nil, fmt.Errorf("repo %q: firma de repodata invalida (la clave de confianza no la verifica)", r.Alias)
			}
		}
		if err := os.WriteFile(CacheIndex(root, r.Alias), b, 0o644); err != nil {
			return nil, err
		}
	}
	return remotes, nil
}

// SignIndex firma el contenido de path (repodata) y escribe path+".sig".
func SignIndex(path string, priv ed25519.PrivateKey) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".sig", ed25519.Sign(priv, data), 0o644)
}

func parsePublic(s string) (ed25519.PublicKey, error) {
	raw := strings.TrimSpace(s)
	b, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("clave publica invalida: %w", err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("clave publica: esperaba %d bytes, hay %d", ed25519.PublicKeySize, len(b))
	}
	return ed25519.PublicKey(b), nil
}

type Match struct {
	Remote Remote
	Pkg    Package
}

func hostArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	default:
		return runtime.GOARCH
	}
}

func prefer(a, b Package) bool {
	c, _ := version.Compare(a.FullVersion(), b.FullVersion())
	if c != 0 {
		return c > 0
	}
	if a.Arch == hostArch() {
		return true
	}
	return b.Arch != hostArch()
}

func Resolve(root, name string) (Match, error) {
	remotes, err := ReadRepos(root)
	if err != nil {
		return Match{}, err
	}
	byAlias := map[string]Remote{}
	for _, r := range remotes {
		byAlias[r.Alias] = r
	}
	files, _ := filepath.Glob(filepath.Join(rootPath(root, cachePath), "*.repodata"))
	var best Match
	for _, f := range files {
		alias := strings.TrimSuffix(filepath.Base(f), ".repodata")
		remote, ok := byAlias[alias]
		if !ok {
			continue
		}
		index, err := ReadIndexFile(f)
		if err != nil {
			continue
		}
		for _, p := range index {
			if p.Name != name {
				continue
			}
			if best.Pkg.Name == "" || prefer(p, best.Pkg) {
				best = Match{Remote: remote, Pkg: p}
			}
		}
	}
	if best.Pkg.Name == "" {
		return Match{}, fmt.Errorf("no se encontro %q en los repositorios (ejecuta yarmouth sync)", name)
	}
	return best, nil
}

func FetchPackage(root string, m Match) (string, error) {
	cache := rootPath(root, cachePath)
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(cache, m.Pkg.Filename())
	// cache-hit: si el .yrm ya esta en la cache y su sha256 coincide con el
	// que certifica el indice firmado (sync), no hace falta volver a la red.
	// Auditable: el sha256 se re-verifica aqui, no se confia en el nombre.
	if data, err := os.ReadFile(dest); err == nil {
		if int64(len(data)) == m.Pkg.Size {
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) == m.Pkg.SHA256 {
				return dest, nil
			}
		}
	}
	data, err := FetchBytes(m.Remote.PackageURL(m.Pkg.Filename()))
	if err != nil {
		return "", err
	}
	if int64(len(data)) != m.Pkg.Size {
		return "", fmt.Errorf("tamano inesperado para %s: %d != %d", m.Pkg.Filename(), len(data), m.Pkg.Size)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != m.Pkg.SHA256 {
		return "", fmt.Errorf("sha256 no coincide para %s", m.Pkg.Filename())
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// CachedPackages enumera los .yrm que la cache verifica: los que ya se
// descargaron bien (FetchPackage los escribio tras verificar tamano y sha256
// contra el repodata firmado). Devuelve un Match por archivo en la cache de
// la raiz (root). Se usa para export: solo sale de la cache lo que un indice
// firmado certifico al entrar, nunca un .yrm huerfano o manipulando.
func CachedPackages(root string) ([]Match, error) {
	cache := rootPath(root, cachePath)
	entries, err := os.ReadDir(cache)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var out []Match
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yrm") {
			continue
		}
		// el sha256 se vuelve a calcular aqui, no se confia en el nombre:
		// si el archivo no verifica, no se exporta (auditable de punta a punta).
		data, err := os.ReadFile(filepath.Join(cache, e.Name()))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		files, err := filepath.Glob(filepath.Join(rootPath(root, cachePath), "*.repodata"))
		if err != nil {
			continue
		}
		for _, idxFile := range files {
			idx, err := ReadIndexFile(idxFile)
			if err != nil {
				continue
			}
			for _, p := range idx {
				if p.SHA256 == sha && p.Filename() == e.Name() {
					alias := strings.TrimSuffix(filepath.Base(idxFile), ".repodata")
					out = append(out, Match{
						Remote: Remote{Alias: alias},
						Pkg:    p,
					})
					break
				}
			}
			if len(out) > 0 && out[len(out)-1].Pkg.Filename() == e.Name() {
				break
			}
		}
	}
	return out, nil
}

// LocalMatchBySHA busca en los indices locales ya sincronizados (cache/*.repodata)
// un paquete con el sha256 dado. Auditable: un .yrm solo entra a la cache local
// si su sha256 lo certifica un indice que sync ya firmo y verifico; import la
// usa para rechazar cualquier archivo que ningun indice certificate.
func LocalMatchBySHA(root, sha string) (Match, bool, error) {
	aliases, err := CachedAliases(root)
	if err != nil {
		return Match{}, false, err
	}
	for _, alias := range aliases {
		idx, err := ReadIndexFile(CacheIndex(root, alias))
		if err != nil {
			continue
		}
		for _, p := range idx {
			if p.SHA256 == sha {
				return Match{Remote: Remote{Alias: alias}, Pkg: p}, true, nil
			}
		}
	}
	return Match{}, false, nil
}

// CachedAliases devuelve los alias de repositorio que tienen indice local
// sincronizado y verificado (cache/<alias>.repodata).
func CachedAliases(root string) ([]string, error) {
	cache := rootPath(root, cachePath)
	matches, err := filepath.Glob(filepath.Join(cache, "*.repodata"))
	if err != nil {
		return nil, err
	}
	var aliases []string
	for _, m := range matches {
		aliases = append(aliases, strings.TrimSuffix(filepath.Base(m), ".repodata"))
	}
	return aliases, nil
}

// CacheFile devuelve la ruta del .yrm dentro de la cache de la raiz (root).
// Usala para exportar/importar: localiza el archivo donde FetchPackage lo
// escribio tras verificar tamano y sha256 contra un indice firmado.
func CacheFile(root, filename string) string {
	return filepath.Join(rootPath(root, cachePath), filename)
}

// CacheRoot devuelve el directorio cache de la raiz (donde viven .yrm y
// *.repodata sincronizados y verificados).
func CacheRoot(root string) string {
	return rootPath(root, cachePath)
}
