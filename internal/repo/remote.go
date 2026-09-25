package repo

import (
	"bytes"
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
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("linea invalida en repos.conf: %q", trimmed)
		}
		remotes = append(remotes, Remote{Alias: strings.TrimSpace(parts[0]), URL: strings.TrimSpace(parts[1])})
	}
	return remotes, nil
}

func WriteRepos(root string, remotes []Remote) error {
	var b strings.Builder
	for _, r := range remotes {
		fmt.Fprintf(&b, "%s\t%s\n", r.Alias, r.URL)
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

func AddRepo(root, alias, url string) error {
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
	return WriteRepos(root, append(remotes, Remote{Alias: alias, URL: url}))
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
		if err := os.WriteFile(CacheIndex(root, r.Alias), b, 0o644); err != nil {
			return nil, err
		}
	}
	return remotes, nil
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
