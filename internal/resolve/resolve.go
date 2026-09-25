package resolve

import (
	"fmt"

	"yarmouth/internal/archive"
	"yarmouth/internal/db"
	"yarmouth/internal/repo"
	"yarmouth/internal/version"
)

type Head struct {
	Name    string
	Match   repo.Match
	Local   string
	Manual  bool
	Upgrade bool
}

type Entry struct {
	Name    string
	Version string
	Match   repo.Match
	Local   string
	Manual  bool
	Upgrade bool
}

// Plan calcula, en orden de dependencias primero, todos los paquetes que hay que
// instalar para cumplir las cabeceras dadas. Las dependencias ya instaladas se
// consideran satisfechas y no se incluyen. Cada nombre aparece una sola vez
// (instala-una-vez), incluso ante ciclos.
func Plan(root string, d *db.DB, heads []Head) ([]Entry, error) {
	p := planner{d: d, root: root, done: map[string]bool{}}
	for _, h := range heads {
		if err := p.expand(h); err != nil {
			return nil, err
		}
	}
	return p.plan, nil
}

type planner struct {
	root string
	d    *db.DB
	done map[string]bool
	plan []Entry
}

func (p *planner) expand(h Head) error {
	name := h.Name
	if !h.Upgrade {
		if _, ok := p.d.Get(name); ok {
			return nil
		}
	}
	if p.done[name] {
		return nil
	}
	p.done[name] = true

	var deps []string
	entry := Entry{Name: name, Match: h.Match, Local: h.Local, Manual: h.Manual, Upgrade: h.Upgrade}
	switch {
	case h.Local != "":
		pkg, err := archive.Open(h.Local)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		deps = append([]string(nil), pkg.Manifest.Depends...)
		entry.Version = pkg.Manifest.Pkgver + "-" + pkg.Manifest.BuildID
	default:
		deps = append([]string(nil), h.Match.Pkg.Depends...)
		entry.Version = h.Match.Pkg.FullVersion()
	}

	for _, dep := range deps {
		if _, ok := p.d.Get(dep); ok {
			continue
		}
		if p.done[dep] {
			continue
		}
		m, err := repo.Resolve(p.root, dep)
		if err != nil {
			return fmt.Errorf("dependencia de %q: %w", name, err)
		}
		if err := p.expand(Head{Name: dep, Match: m}); err != nil {
			return err
		}
	}
	p.plan = append(p.plan, entry)
	return nil
}

// UpgradePlan prepara la actualizacion de los paquetes indicados (o de todos los
// instalados si names esta vacio) a la versión mas reciente disponible en los
// repositorios, agregando como auto las dependencias nuevas que falten.
// explicit indica que names fue pedido explícitamente: un paquete ausente de los
// repositorios es un error.
func UpgradePlan(root string, d *db.DB, names []string, explicit bool) ([]Entry, []string, error) {
	if len(names) == 0 {
		for _, r := range d.All() {
			names = append(names, r.Name)
		}
	}
	var heads []Head
	var skipped []string
	for _, name := range names {
		rec, ok := d.Get(name)
		if !ok {
			return nil, nil, fmt.Errorf("paquete %q no esta instalado", name)
		}
		m, err := repo.Resolve(root, name)
		if err != nil {
			if explicit {
				return nil, nil, fmt.Errorf("paquete %q: %w", name, err)
			}
			skipped = append(skipped, fmt.Sprintf("%s: no en los repositorios", name))
			continue
		}
		cmp, _ := version.Compare(m.Pkg.FullVersion(), rec.FullVersion())
		if cmp <= 0 {
			skipped = append(skipped, fmt.Sprintf("%s: ya actualizado (%s)", name, rec.FullVersion()))
			continue
		}
		heads = append(heads, Head{Name: name, Match: m, Manual: rec.Manual, Upgrade: true})
	}
	entries, err := Plan(root, d, heads)
	if err != nil {
		return nil, nil, err
	}
	// los paquetes auto instalados como dependencias nuevas no se marcan world
	for i := range entries {
		if !entries[i].Upgrade {
			entries[i].Manual = false
		}
	}
	return entries, skipped, nil
}
