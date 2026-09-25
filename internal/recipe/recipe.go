package recipe

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
	"yarmouth/internal/archive"
	"yarmouth/internal/metadata"
	"yarmouth/internal/repo"
)

type Recipe struct {
	Manifest metadata.Manifest
	Sources  []string
	SHA256   []string
	Prepare  string
	Build    string
	Install  string
	Hooks    archive.Hooks
}

func Read(r io.Reader) (*Recipe, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	rec := &Recipe{}
	var lines []string
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, " = ") {
			return nil, fmt.Errorf("linea %d: falta separador \" = \"", i+1)
		}
		parts := strings.SplitN(line, " = ", 2)
		key, val := parts[0], parts[1]
		if key == "" || val == "" {
			return nil, fmt.Errorf("linea %d: clave o valor vacios", i+1)
		}
		lines = append(lines, key+" = "+val)
		switch key {
		case "pkgname":
			rec.Manifest.Pkgname = val
		case "pkgver":
			rec.Manifest.Pkgver = val
		case "buildid":
			rec.Manifest.BuildID = val
		case "arch":
			rec.Manifest.Arch = val
		case "description":
			rec.Manifest.Desc = val
		case "license":
			rec.Manifest.License = val
		case "homepage":
			rec.Manifest.Homepage = val
		case "maintainer":
			rec.Manifest.Maintainer = val
		case "depends":
			rec.Manifest.Depends = append(rec.Manifest.Depends, val)
		case "sources":
			rec.Sources = append(rec.Sources, val)
		case "sha256":
			rec.SHA256 = append(rec.SHA256, val)
		case "prepare":
			rec.Prepare = appendLine(rec.Prepare, val)
		case "build":
			rec.Build = appendLine(rec.Build, val)
		case "install":
			rec.Install = appendLine(rec.Install, val)
		case "pre-install":
			rec.Hooks.PreInstall = []byte(val)
		case "post-install":
			rec.Hooks.PostInstall = []byte(val)
		case "pre-remove":
			rec.Hooks.PreRemove = []byte(val)
		case "post-remove":
			rec.Hooks.PostRemove = []byte(val)
		default:
			return nil, fmt.Errorf("linea %d: clave desconocida %q", i+1, key)
		}
	}
	if err := rec.validate(); err != nil {
		return nil, err
	}
	return rec, nil
}

func ReadFile(path string) (*Recipe, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

func appendLine(a, b string) string {
	if a == "" {
		return b
	}
	return a + "\n" + b
}

func (r *Recipe) validate() error {
	if r.Manifest.Pkgname == "" || r.Manifest.Pkgver == "" || r.Manifest.BuildID == "" || r.Manifest.Arch == "" {
		return errors.New("receta incompleta: faltan pkgname, pkgver, buildid o arch")
	}
	if err := r.Manifest.Validate(); err != nil {
		return err
	}
	if len(r.Sources) == 0 {
		return errors.New("la receta no declara sources")
	}
	if len(r.SHA256) != len(r.Sources) {
		return fmt.Errorf("sha256 debe declararse una vez por fuente (%d fuentes, %d hashes)", len(r.Sources), len(r.SHA256))
	}
	for _, h := range r.SHA256 {
		if len(h) != 64 {
			return errors.New("los sha256 deben tener 64 caracteres hex")
		}
	}
	return nil
}

// Build ejecuta la receta en un directorio temporal y escribe el .yrm en outDir.
// Devuelve la ruta del paquete generado.
func Build(path, outDir string) (string, error) {
	rec, err := ReadFile(path)
	if err != nil {
		return "", err
	}
	resolveSources(rec, filepath.Dir(path))
	tmp, err := os.MkdirTemp("", "yarmouth-build-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(tmp)
		}
	}()

	srcroot := filepath.Join(tmp, "src")
	if err := os.MkdirAll(srcroot, 0o755); err != nil {
		return "", err
	}
	srcdir, err := fetchSources(rec, srcroot)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(tmp, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	env := buildEnv(srcroot, srcdir, dest, rec)
	if rec.Prepare != "" {
		if err := runStep(&rec.Prepare, "prepare", srcdir, env); err != nil {
			return "", err
		}
	}
	if rec.Build != "" {
		if err := runStep(&rec.Build, "build", srcdir, env); err != nil {
			return "", err
		}
	}
	if rec.Install != "" {
		if err := runStep(&rec.Install, "install", srcdir, env); err != nil {
			return "", err
		}
	}

	name := fmt.Sprintf("%s-%s-%s.%s.yrm", rec.Manifest.Pkgname, rec.Manifest.Pkgver, rec.Manifest.BuildID, rec.Manifest.Arch)
	out := filepath.Join(outDir, name)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	_, err = archive.Create(f, archive.CreateOptions{Manifest: rec.Manifest, DestDir: dest, Hooks: rec.Hooks})
	cerr := f.Close()
	if err != nil {
		os.Remove(out)
		return "", err
	}
	if cerr != nil {
		os.Remove(out)
		return "", cerr
	}
	ok = true
	return out, nil
}

// resolveSources convierte las rutas relativas en rutas absolutas respecto al
// directorio de la receta; las urls http(s) no se tocan.
func resolveSources(rec *Recipe, base string) {
	for i, src := range rec.Sources {
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			continue
		}
		p := strings.TrimPrefix(src, "file://")
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		rec.Sources[i] = p
	}
}

func fetchSources(rec *Recipe, srcroot string) (string, error) {
	for i, src := range rec.Sources {
		name := filepath.Base(strings.TrimSuffix(strings.TrimPrefix(src, "file://"), "/"))
		if name == "." || name == "/" || name == "" {
			return "", fmt.Errorf("fuente invalida: %s", src)
		}
		data, err := repo.FetchBytes(src)
		if err != nil {
			return "", fmt.Errorf("descargando %s: %w", src, err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != rec.SHA256[i] {
			return "", fmt.Errorf("sha256 de %s no coincide: %s != %s", src, got, rec.SHA256[i])
		}
		local := filepath.Join(srcroot, name)
		tarball := false
		switch {
		case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
			tarball = true
		case strings.HasSuffix(name, ".tar.xz"), strings.HasSuffix(name, ".txz"):
			tarball = true
		case strings.HasSuffix(name, ".tar.bz2"), strings.HasSuffix(name, ".tbz2"):
			tarball = true
		case strings.HasSuffix(name, ".tar"):
			tarball = true
		case strings.HasSuffix(name, ".zip"):
			tarball = true
		}
		if err := os.WriteFile(local, data, 0o644); err != nil {
			return "", err
		}
		if tarball {
			if err := extractArchive(local); err != nil {
				return "", fmt.Errorf("%s: %w", src, err)
			}
		}
	}
	return detectSrcdir(srcroot)
}

func extractArchive(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	root := filepath.Dir(path)
	switch {
	case strings.HasSuffix(path, ".tar.gz"), strings.HasSuffix(path, ".tgz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		return extractTar(gz, root)
	case strings.HasSuffix(path, ".tar.xz"), strings.HasSuffix(path, ".txz"):
		xr, err := xz.NewReader(f)
		if err != nil {
			return err
		}
		return extractTar(xr, root)
	case strings.HasSuffix(path, ".tar.bz2"), strings.HasSuffix(path, ".tbz2"):
		return extractTar(bzip2.NewReader(f), root)
	case strings.HasSuffix(path, ".tar"):
		return extractTar(f, root)
	case strings.HasSuffix(path, ".zip"):
		return extractZip(path, root)
	}
	return errors.New("formato de archivo no soportado")
}

func extractTar(r io.Reader, root string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(root, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			cerr := out.Close()
			if err != nil || cerr != nil {
				return errors.New("archivo corrupto en el archivo fuente")
			}
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			link, err := safeJoin(root, hdr.Linkname)
			if err != nil {
				return err
			}
			if err := os.Link(link, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("tipo de entrada no soportado: %s", hdr.Name)
		}
	}
	return nil
}

func extractZip(path, root string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		target, err := safeJoin(root, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		cerr := out.Close()
		if err != nil || cerr != nil {
			return errors.New("archivo zip corrupto")
		}
	}
	return nil
}

func safeJoin(root, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("ruta no segura en el archivo fuente: %q", name)
	}
	return filepath.Join(root, clean), nil
}

// detectSrcdir devuelve el directorio de construccion: si las fuentes dejaron
// un unico directorio al nivel superior, ese; si no, srcroot.
func detectSrcdir(srcroot string) (string, error) {
	entries, err := os.ReadDir(srcroot)
	if err != nil {
		return "", err
	}
	dirs := 0
	last := ""
	for _, e := range entries {
		if e.IsDir() {
			dirs++
			last = e.Name()
		}
	}
	if dirs == 1 {
		return filepath.Join(srcroot, last), nil
	}
	return srcroot, nil
}

func buildEnv(srcroot, srcdir, dest string, rec *Recipe) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		"PKGNAME="+rec.Manifest.Pkgname,
		"PKGVER="+rec.Manifest.Pkgver,
		"BUILDID="+rec.Manifest.BuildID,
		"ARCH="+rec.Manifest.Arch,
		"PREFIX=/usr",
		"SRCROOT="+srcroot,
		"SRCDIR="+srcdir,
		"DESTDIR="+dest,
	)
	return env
}

func runStep(script *string, label, srcdir string, env []string) error {
	cmd := exec.Command("/bin/bash", "-e", "-c", *script)
	cmd.Dir = srcdir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("paso %q fallo: %w", label, err)
	}
	return nil
}
