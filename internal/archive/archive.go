package archive

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ulikunitz/xz"
	"yarmouth/internal/metadata"
)

const ManifestPath = "yarmouth/manifest"

const (
	HookPreInstall  = "yarmouth/pre-install"
	HookPostInstall = "yarmouth/post-install"
	HookPreRemove   = "yarmouth/pre-remove"
	HookPostRemove  = "yarmouth/post-remove"
)

type Hooks struct {
	PreInstall  []byte
	PostInstall []byte
	PreRemove   []byte
	PostRemove  []byte
}

type CreateOptions struct {
	Manifest metadata.Manifest
	DestDir  string
	Hooks    Hooks
}

type CreateResult struct {
	DataHash string
	Files    []metadata.FileEntry
}

func Create(out io.Writer, opts CreateOptions) (CreateResult, error) {
	var res CreateResult
	if opts.Manifest.Pkgname == "" || opts.Manifest.Pkgver == "" || opts.Manifest.BuildID == "" || opts.Manifest.Arch == "" {
		return res, errors.New("manifest incompleto: faltan pkgname, pkgver, buildid o arch")
	}
	info, err := os.Stat(opts.DestDir)
	if err != nil {
		return res, fmt.Errorf("destdir: %w", err)
	}
	if !info.IsDir() {
		return res, fmt.Errorf("destdir %q no es un directorio", opts.DestDir)
	}

	xw, err := xz.NewWriter(out)
	if err != nil {
		return res, err
	}
	tw := tar.NewWriter(xw)

	agg := sha256.New()

	hooks := []struct {
		name string
		data []byte
	}{
		{HookPreInstall, opts.Hooks.PreInstall},
		{HookPostInstall, opts.Hooks.PostInstall},
		{HookPreRemove, opts.Hooks.PreRemove},
		{HookPostRemove, opts.Hooks.PostRemove},
	}
	for _, h := range hooks {
		if len(h.data) == 0 {
			continue
		}
		if err := writePayload(tw, agg, &res.Files, h.name, 0o755, bytes.NewReader(h.data), int64(len(h.data))); err != nil {
			closeWriters(tw, xw)
			return res, err
		}
	}

	if err := walkPayload(tw, agg, &res.Files, opts.DestDir); err != nil {
		closeWriters(tw, xw)
		return res, err
	}

	m := opts.Manifest
	m.Files = res.Files
	m.DataHash = hex.EncodeToString(agg.Sum(nil))
	var mb bytes.Buffer
	if err := metadata.Write(&mb, m); err != nil {
		closeWriters(tw, xw)
		return res, err
	}
	hdr := &tar.Header{Name: ManifestPath, Mode: 0o644, Size: int64(mb.Len()), Typeflag: tar.TypeReg, Uid: 0, Gid: 0, ModTime: time.Unix(0, 0)}
	if err := tw.WriteHeader(hdr); err != nil {
		closeWriters(tw, xw)
		return res, err
	}
	if _, err := tw.Write(mb.Bytes()); err != nil {
		closeWriters(tw, xw)
		return res, err
	}
	if err := tw.Close(); err != nil {
		return res, err
	}
	if err := xw.Close(); err != nil {
		return res, err
	}
	res.DataHash = m.DataHash
	return res, nil
}

func closeWriters(tw *tar.Writer, xw *xz.Writer) {
	tw.Close()
	xw.Close()
}

func writePayload(tw *tar.Writer, agg hash.Hash, files *[]metadata.FileEntry, name string, mode int64, r io.Reader, size int64) error {
	fh := sha256.New()
	hdr := &tar.Header{Name: name, Mode: mode, Size: size, Typeflag: tar.TypeReg, Uid: 0, Gid: 0, ModTime: time.Unix(0, 0)}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.CopyN(io.MultiWriter(tw, agg, fh), r, size); err != nil {
		return err
	}
	*files = append(*files, metadata.FileEntry{SHA256: hex.EncodeToString(fh.Sum(nil)), Path: name})
	return nil
}

type inodeKey struct {
	dev uint64
	ino uint64
}

func walkPayload(tw *tar.Writer, agg hash.Hash, files *[]metadata.FileEntry, root string) error {
	seen := map[inodeKey]string{}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		fi, err := d.Info()
		if err != nil {
			return err
		}
		switch d.Type() {
		case fs.ModeDir:
			hdr := &tar.Header{Name: name + "/", Mode: int64(fi.Mode().Perm()) | 0o200, Typeflag: tar.TypeDir, Uid: 0, Gid: 0, ModTime: time.Unix(0, 0)}
			return tw.WriteHeader(hdr)
		case fs.ModeSymlink:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hdr := &tar.Header{Name: name, Mode: int64(fi.Mode().Perm()), Typeflag: tar.TypeSymlink, Linkname: link, Uid: 0, Gid: 0, ModTime: time.Unix(0, 0)}
			return tw.WriteHeader(hdr)
		case 0:
			st := fi.Sys().(*syscall.Stat_t)
			if st.Nlink > 1 {
				k := inodeKey{uint64(st.Dev), uint64(st.Ino)}
				if first, ok := seen[k]; ok {
					hdr := &tar.Header{Name: name, Mode: int64(fi.Mode().Perm()), Typeflag: tar.TypeLink, Linkname: first, Uid: 0, Gid: 0, ModTime: time.Unix(0, 0)}
					return tw.WriteHeader(hdr)
				}
				seen[k] = name
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			err = writePayload(tw, agg, files, name, int64(fi.Mode().Perm()), f, fi.Size())
			f.Close()
			return err
		default:
			return fmt.Errorf("tipo de archivo no soportado: %s (%s)", path, d.Type())
		}
	})
}

type Entry struct {
	Name     string
	Size     int64
	Mode     int64
	Typeflag byte
	Linkname string
}

type Package struct {
	Path     string
	Manifest metadata.Manifest
	Entries  []Entry
	SHA256   string
	Hooks    map[string][]byte
	hashes   map[string]string
}

func (p *Package) Hash(name string) string { return p.hashes[name] }

func Open(path string) (*Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := xz.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("paquete invalido: %w", err)
	}
	tr := tar.NewReader(zr)
	pkg := &Package{Path: path, hashes: map[string]string{}, Hooks: map[string][]byte{}}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("paquete corrupto: %w", err)
		}
		if hdr.Name == ManifestPath {
			var b bytes.Buffer
			if _, err := io.Copy(&b, tr); err != nil {
				return nil, err
			}
			m, err := metadata.Read(&b)
			if err != nil {
				return nil, fmt.Errorf("manifest invalido: %w", err)
			}
			pkg.Manifest = m
			continue
		}
		pkg.Entries = append(pkg.Entries, Entry{Name: hdr.Name, Size: hdr.Size, Mode: hdr.Mode, Typeflag: hdr.Typeflag, Linkname: hdr.Linkname})
		if pkg.IsHook(hdr.Name) {
			var b bytes.Buffer
			if _, err := io.Copy(&b, tr); err != nil {
				return nil, err
			}
			pkg.Hooks[hdr.Name] = b.Bytes()
		}
	}
	if pkg.Manifest.Pkgname == "" {
		return nil, errors.New("falta el manifest en el paquete")
	}
	for _, fe := range pkg.Manifest.Files {
		pkg.hashes[fe.Path] = fe.SHA256
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	pkg.SHA256 = hex.EncodeToString(sum[:])
	return pkg, nil
}

func (p *Package) VerifyContent() error {
	f, err := os.Open(p.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := xz.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	agg := sha256.New()
	content := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("verificacion: %w", err)
		}
		if hdr.Name == ManifestPath {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		fh := sha256.New()
		n, err := io.Copy(io.MultiWriter(agg, fh), tr)
		if err != nil {
			return err
		}
		if n != hdr.Size {
			return fmt.Errorf("tamano inconsistente en %s", hdr.Name)
		}
		got := hex.EncodeToString(fh.Sum(nil))
		want := p.hashes[hdr.Name]
		if want == "" {
			return fmt.Errorf("archivo sin hash declarado: %s", hdr.Name)
		}
		if got != want {
			return fmt.Errorf("hash no coincide en %s", hdr.Name)
		}
		content[hdr.Name] = true
	}
	for _, fe := range p.Manifest.Files {
		if !content[fe.Path] {
			return fmt.Errorf("falta el archivo declarado: %s", fe.Path)
		}
	}
	want := p.Manifest.DataHash
	if want == "" {
		return errors.New("sin datahash en el manifest")
	}
	if got := hex.EncodeToString(agg.Sum(nil)); got != want {
		return fmt.Errorf("datahash no coincide: %s != %s", got, want)
	}
	return nil
}

func (p *Package) IsHook(name string) bool {
	switch name {
	case HookPreInstall, HookPostInstall, HookPreRemove, HookPostRemove:
		return true
	}
	return false
}

func RootPath(root, name string) string {
	clean := filepath.Clean("/" + name)
	if root == "" || root == "/" {
		return clean
	}
	return filepath.Join(root, clean)
}

func (p *Package) Extract(root string, skip func(name string) bool) ([]string, error) {
	f, err := os.Open(p.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := xz.NewReader(f)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	var created []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			rollbackExtract(created)
			return nil, err
		}
		if skip != nil && skip(hdr.Name) {
			continue
		}
		target := RootPath(root, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			mode := os.FileMode(hdr.Mode) | 0o200
			if err := os.MkdirAll(target, mode); err != nil {
				rollbackExtract(created)
				return nil, err
			}
			created = append(created, target)
		case tar.TypeSymlink:
			if err := removeExisting(target); err != nil {
				rollbackExtract(created)
				return nil, err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				rollbackExtract(created)
				return nil, err
			}
			lchownIfRoot(target, hdr)
			created = append(created, target)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				rollbackExtract(created)
				return nil, err
			}
			tmp, err := os.CreateTemp(filepath.Dir(target), ".yrm-tmp-")
			if err != nil {
				rollbackExtract(created)
				return nil, err
			}
			n, err := io.Copy(tmp, tr)
			if err == nil && n != hdr.Size {
				err = fmt.Errorf("tamano inconsistente en %s", hdr.Name)
			}
			if err == nil {
				err = tmp.Chmod(os.FileMode(hdr.Mode))
			}
			if err == nil {
				err = tmp.Close()
			}
			if err != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				rollbackExtract(created)
				return nil, err
			}
			if err := removeExisting(target); err != nil {
				rollbackExtract(created)
				return nil, err
			}
			if err := os.Rename(tmp.Name(), target); err != nil {
				os.Remove(tmp.Name())
				rollbackExtract(created)
				return nil, err
			}
			lchownIfRoot(target, hdr)
			created = append(created, target)
		case tar.TypeLink:
			if err := removeExisting(target); err != nil {
				rollbackExtract(created)
				return nil, err
			}
			if err := os.Link(RootPath(root, hdr.Linkname), target); err != nil {
				rollbackExtract(created)
				return nil, err
			}
			created = append(created, target)
		default:
			rollbackExtract(created)
			return nil, fmt.Errorf("tipo de entrada no soportado: %s", hdr.Name)
		}
	}
	return created, nil
}

func removeExisting(target string) error {
	err := os.Remove(target)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func lchownIfRoot(target string, hdr *tar.Header) {
	if os.Geteuid() != 0 {
		return
	}
	os.Lchown(target, hdr.Uid, hdr.Gid)
}

func rollbackExtract(created []string) {
	for i := len(created) - 1; i >= 0; i-- {
		os.Remove(created[i])
	}
}
