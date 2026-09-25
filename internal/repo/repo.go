package repo

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"yarmouth/internal/version"
)

type Package struct {
	Name    string
	Version string
	BuildID string
	Arch    string
	SHA256  string
	Size    int64
	Depends []string
}

func (p Package) FullVersion() string { return p.Version + "-" + p.BuildID }

func (p Package) Filename() string {
	return p.Name + "-" + p.Version + "-" + p.BuildID + "." + p.Arch + ".yrm"
}

const HeaderLine = "# yarmouth repodata v1"

func WriteIndex(w io.Writer, pkgs []Package) error {
	sorted := append([]Package(nil), pkgs...)
	sortPackages(sorted)
	if _, err := io.WriteString(w, HeaderLine+"\n"); err != nil {
		return err
	}
	for _, p := range sorted {
		line := strings.Join([]string{
			p.Name,
			p.FullVersion(),
			p.Arch,
			p.SHA256,
			strconv.FormatInt(p.Size, 10),
			strings.Join(p.Depends, ","),
		}, "\t")
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	return nil
}

func ReadIndex(r io.Reader) ([]Package, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 6 {
			return nil, io.ErrUnexpectedEOF
		}
		size, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(fields[1], "-")
		mver := strings.Join(parts[:len(parts)-1], "-")
		buildid := parts[len(parts)-1]
		var depends []string
		if fields[5] != "" {
			depends = strings.Split(fields[5], ",")
		}
		pkgs = append(pkgs, Package{
			Name:    fields[0],
			Version: mver,
			BuildID: buildid,
			Arch:    fields[2],
			SHA256:  fields[3],
			Size:    size,
			Depends: depends,
		})
	}
	return pkgs, nil
}

func WriteIndexFile(path string, pkgs []Package) error {
	var buf bytes.Buffer
	if err := WriteIndex(&buf, pkgs); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func sortPackages(pkgs []Package) {
	for i := 1; i < len(pkgs); i++ {
		for j := i; j > 0; j-- {
			a, b := pkgs[j], pkgs[j-1]
			if a.Name != b.Name {
				if a.Name < b.Name {
					pkgs[j], pkgs[j-1] = pkgs[j-1], pkgs[j]
					continue
				}
				break
			}
			ca, err := version.Compare(a.FullVersion(), b.FullVersion())
			if err != nil {
				break
			}
			if ca < 0 || (ca == 0 && a.Arch < b.Arch) {
				pkgs[j], pkgs[j-1] = pkgs[j-1], pkgs[j]
				continue
			}
			break
		}
	}
}
