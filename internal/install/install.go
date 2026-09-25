package install

import (
	"archive/tar"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"yarmouth/internal/archive"
	"yarmouth/internal/db"
)

type Options struct {
	Force bool
}

func Install(pkg *archive.Package, d *db.DB, root string, opts Options) error {
	name := pkg.Manifest.Pkgname
	if _, ok := d.Get(name); ok {
		return fmt.Errorf("paquete %q ya esta instalado", name)
	}
	if err := pkg.VerifyContent(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	owner := d.OwnerMap()
	if err := checkConflicts(pkg, owner, root, opts.Force); err != nil {
		return err
	}
	if err := runHook("pre-install", pkg.Hooks[archive.HookPreInstall], root, name, pkg.Manifest.Pkgver); err != nil {
		return err
	}
	created, err := pkg.Extract(root, skipControl)
	if err != nil {
		return err
	}
	rec := recordFromPackage(pkg)
	if err := d.Add(rec); err != nil {
		rollbackPaths(root, created)
		return err
	}
	if err := runHook("post-install", pkg.Hooks[archive.HookPostInstall], root, name, pkg.Manifest.Pkgver); err != nil {
		return err
	}
	return nil
}

func RemovePackage(name string, d *db.DB, root string, opts Options) error {
	_, err := removeByName(name, d, root, opts)
	return err
}

func removeByName(name string, d *db.DB, root string, opts Options) (string, error) {
	if name == "" {
		return "", errors.New("falta el nombre del paquete")
	}
	rec, ok := d.Get(name)
	if !ok {
		return "", fmt.Errorf("paquete %q no esta instalado", name)
	}
	if !opts.Force {
		for _, other := range d.All() {
			if other.Name == name {
				continue
			}
			for _, dep := range other.Depends {
				if dep == name {
					return "", fmt.Errorf("paquete %q es requerido por %q (usa -f para forzar)", name, other.Name)
				}
			}
		}
	}
	if err := runHook("pre-remove", rec.Hooks[archive.HookPreRemove], root, name, rec.Pkgver); err != nil {
		return "", err
	}
	owner := d.OwnerMap()
	for _, f := range rec.Files {
		if owner[f.Path] == name {
			os.Remove(archive.RootPath(root, f.Path))
		}
	}
	for _, s := range rec.Symlinks {
		if owner[s.Path] == name {
			os.Remove(archive.RootPath(root, s.Path))
		}
	}
	for hp := range rec.Hardlinks {
		if owner[hp] == name {
			os.Remove(archive.RootPath(root, hp))
		}
	}
	dirs := append([]string(nil), rec.Dirs...)
	sort.Slice(dirs, func(i, j int) bool { return strings.Count(dirs[i], "/") > strings.Count(dirs[j], "/") })
	for _, dr := range dirs {
		if owner[dr] == name {
			os.Remove(archive.RootPath(root, dr))
		}
	}
	if err := runHook("post-remove", rec.Hooks[archive.HookPostRemove], root, name, rec.Pkgver); err != nil {
		return fullVersion(rec), err
	}
	if err := d.Remove(name); err != nil {
		return "", err
	}
	return fullVersion(rec), nil
}

func fullVersion(r *db.Record) string { return r.Pkgver + "-" + r.BuildID }

func skipControl(name string) bool { return strings.HasPrefix(name, "yarmouth/") }

func payloadEntries(pkg *archive.Package) []archive.Entry {
	var out []archive.Entry
	for _, e := range pkg.Entries {
		if !skipControl(e.Name) {
			out = append(out, e)
		}
	}
	return out
}

func checkConflicts(pkg *archive.Package, owner map[string]string, root string, force bool) error {
	name := pkg.Manifest.Pkgname
	for _, e := range payloadEntries(pkg) {
		target := archive.RootPath(root, e.Name)
		fi, err := os.Lstat(target)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if e.Typeflag == tar.TypeDir {
			if !fi.IsDir() {
				return fmt.Errorf("conflicto: %s existe y no es un directorio", target)
			}
			continue
		}
		own := owner[e.Name]
		switch {
		case own != "" && own != name && !force:
			return fmt.Errorf("conflicto: %s pertenece a %q", target, own)
		case own == "" && !force:
			return fmt.Errorf("conflicto: %s no esta registrado por ningun paquete", target)
		}
	}
	return nil
}

func recordFromPackage(pkg *archive.Package) *db.Record {
	m := pkg.Manifest
	rec := &db.Record{
		Name:      m.Pkgname,
		Pkgver:    m.Pkgver,
		BuildID:   m.BuildID,
		Arch:      m.Arch,
		SHA256:    pkg.SHA256,
		DataHash:  m.DataHash,
		Depends:   append([]string(nil), m.Depends...),
		Hardlinks: map[string]string{},
		Hooks:     map[string][]byte{},
		Manual:    true,
	}
	for k, v := range pkg.Hooks {
		rec.Hooks[k] = v
	}
	for _, e := range payloadEntries(pkg) {
		switch e.Typeflag {
		case tar.TypeReg:
			rec.Files = append(rec.Files, db.FileEntry{Path: e.Name, SHA256: pkg.Hash(e.Name)})
		case tar.TypeSymlink:
			rec.Symlinks = append(rec.Symlinks, db.SymlinkEntry{Path: e.Name, Target: e.Linkname})
		case tar.TypeDir:
			rec.Dirs = append(rec.Dirs, strings.TrimSuffix(e.Name, "/"))
		case tar.TypeLink:
			rec.Hardlinks[e.Name] = e.Linkname
		}
	}
	return rec
}

func runHook(label string, script []byte, root, name, version string) error {
	if len(script) == 0 {
		return nil
	}
	cmd := exec.Command("/bin/sh", "-c", string(script))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"YK_ROOT="+root,
		"YK_PKGNAME="+name,
		"YK_PKGVERSION="+version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook %s de %q fallo: %w", label, name, err)
	}
	return nil
}

func rollbackPaths(root string, created []string) {
	for i := len(created) - 1; i >= 0; i-- {
		os.Remove(created[i])
	}
}
