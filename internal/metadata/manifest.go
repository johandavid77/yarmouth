package metadata

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"yarmouth/internal/version"
)

type FileEntry struct {
	SHA256 string
	Path   string
}

type Manifest struct {
	Pkgname    string
	Pkgver     string
	BuildID    string
	Arch       string
	Desc       string
	License    string
	Homepage   string
	Maintainer string
	Depends    []string
	Files      []FileEntry
	DataHash   string
}

func (m Manifest) Validate() error { return m.validate() }

func (m Manifest) validate() error {
	if m.Pkgname == "" || m.Pkgver == "" || m.BuildID == "" || m.Arch == "" {
		return errors.New("manifest incompleto: faltan pkgname, pkgver, buildid o arch")
	}
	if _, err := version.Parse(m.Pkgver); err != nil {
		return fmt.Errorf("pkgver invalido: %w", err)
	}
	if m.DataHash != "" && len(m.DataHash) != 64 {
		return errors.New("datahash debe ser un sha256 hex (64 caracteres)")
	}
	for _, f := range m.Files {
		if len(f.SHA256) != 64 || f.Path == "" {
			return fmt.Errorf("entrada file invalida: %q", f.Path)
		}
	}
	return nil
}

func Read(r io.Reader) (Manifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{}
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, " = ") {
			return Manifest{}, fmt.Errorf("linea %d: falta separador \" = \"", i+1)
		}
		parts := strings.SplitN(line, " = ", 2)
		key, val := parts[0], parts[1]
		if key == "" || val == "" {
			return Manifest{}, fmt.Errorf("linea %d: clave o valor vacios", i+1)
		}
		switch key {
		case "pkgname":
			m.Pkgname = val
		case "pkgver":
			m.Pkgver = val
		case "buildid":
			m.BuildID = val
		case "arch":
			m.Arch = val
		case "description":
			m.Desc = val
		case "license":
			m.License = val
		case "homepage":
			m.Homepage = val
		case "maintainer":
			m.Maintainer = val
		case "depends":
			m.Depends = append(m.Depends, val)
		case "file":
			pieces := strings.Fields(val)
			if len(pieces) != 2 {
				return Manifest{}, fmt.Errorf("linea %d: file debe ser \"<sha256> <ruta>\"", i+1)
			}
			m.Files = append(m.Files, FileEntry{SHA256: pieces[0], Path: pieces[1]})
		case "datahash":
			m.DataHash = val
		default:
			return Manifest{}, fmt.Errorf("linea %d: clave desconocida %q", i+1, key)
		}
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func ReadFile(path string) (Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	return Read(f)
}

func Write(w io.Writer, m Manifest) error {
	if err := m.validate(); err != nil {
		return err
	}
	lines := []string{
		"pkgname = " + m.Pkgname,
		"pkgver = " + m.Pkgver,
		"buildid = " + m.BuildID,
		"arch = " + m.Arch,
	}
	if m.Desc != "" {
		lines = append(lines, "description = "+m.Desc)
	}
	if m.License != "" {
		lines = append(lines, "license = "+m.License)
	}
	if m.Homepage != "" {
		lines = append(lines, "homepage = "+m.Homepage)
	}
	if m.Maintainer != "" {
		lines = append(lines, "maintainer = "+m.Maintainer)
	}
	depends := append([]string(nil), m.Depends...)
	sort.Strings(depends)
	for _, d := range depends {
		lines = append(lines, "depends = "+d)
	}
	files := append([]FileEntry(nil), m.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		lines = append(lines, "file = "+f.SHA256+" "+f.Path)
	}
	if m.DataHash != "" {
		lines = append(lines, "datahash = "+m.DataHash)
	}
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}
