package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type FileEntry struct {
	Path   string
	SHA256 string
}

type SymlinkEntry struct {
	Path   string
	Target string
}

type Record struct {
	Name      string
	Pkgver    string
	BuildID   string
	Arch      string
	SHA256    string
	DataHash  string
	Depends   []string
	Files     []FileEntry
	Dirs      []string
	Symlinks  []SymlinkEntry
	Hardlinks map[string]string
	Hooks     map[string][]byte
	Manual    bool
}

func (r Record) FullVersion() string { return r.Pkgver + "-" + r.BuildID }

type DB struct {
	root  string
	path  string
	pkgs  map[string]*Record
	order []string
}

const dbPath = "var/db/yarmouth/pkgdb.json"

const Format = 1

func Path(root string) string {
	if root == "" || root == "/" {
		return "/" + dbPath
	}
	return filepath.Join(root, dbPath)
}

func Open(root string) (*DB, error) {
	d := &DB{root: root, path: Path(root), pkgs: map[string]*Record{}}
	data, err := os.ReadFile(d.path)
	if errors.Is(err, fs.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	doc := struct {
		Format   int
		Packages []Record
	}{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("pkgdb corrupto: %w", err)
	}
	if doc.Format != Format {
		return nil, fmt.Errorf("formato de pkgdb no soportado: %d", doc.Format)
	}
	for i := range doc.Packages {
		r := &doc.Packages[i]
		d.pkgs[r.Name] = r
		d.order = append(d.order, r.Name)
	}
	return d, nil
}

func (d *DB) Add(r *Record) error {
	if _, ok := d.pkgs[r.Name]; ok {
		return fmt.Errorf("paquete %q ya esta instalado", r.Name)
	}
	d.pkgs[r.Name] = r
	d.order = append(d.order, r.Name)
	return d.save()
}

func (d *DB) Remove(name string) error {
	if _, ok := d.pkgs[name]; !ok {
		return fmt.Errorf("paquete %q no esta instalado", name)
	}
	delete(d.pkgs, name)
	for i, n := range d.order {
		if n == name {
			d.order = append(d.order[:i], d.order[i+1:]...)
			break
		}
	}
	return d.save()
}

func (d *DB) Get(name string) (*Record, bool) {
	r, ok := d.pkgs[name]
	return r, ok
}

func (d *DB) All() []Record {
	recs := make([]Record, 0, len(d.pkgs))
	for _, n := range d.order {
		recs = append(recs, *d.pkgs[n])
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Name < recs[j].Name })
	return recs
}

func (d *DB) OwnerMap() map[string]string {
	owner := map[string]string{}
	for _, r := range d.All() {
		for _, f := range r.Files {
			owner[f.Path] = r.Name
		}
		for _, s := range r.Symlinks {
			owner[s.Path] = r.Name
		}
		for _, dir := range r.Dirs {
			owner[dir] = r.Name
		}
		for hp := range r.Hardlinks {
			owner[hp] = r.Name
		}
	}
	return owner
}

func (d *DB) save() error {
	doc := struct {
		Format   int
		Packages []Record
	}{Format: Format, Packages: []Record{}}
	for _, n := range d.order {
		doc.Packages = append(doc.Packages, *d.pkgs[n])
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(d.path), 0o755); err != nil {
		return err
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, d.path)
}
