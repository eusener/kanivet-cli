package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"gopkg.in/yaml.v3"

	catalogdata "github.com/eusener/kanivet-cli/catalog"
)

type rootFile struct {
	Schema     int      `yaml:"schema"`
	Version    string   `yaml:"version"`
	Categories []string `yaml:"categories"`
}

// Embedded carrega o catálogo que vem dentro do binário.
func Embedded() (*Catalog, error) { return Load(catalogdata.FS) }

// Load lê catalog.yaml, categories/, items/ e profiles/ (um objeto por arquivo).
func Load(fsys fs.FS) (*Catalog, error) {
	var root rootFile
	if err := decode(fsys, "catalog.yaml", &root); err != nil {
		return nil, err
	}
	if root.Schema > SchemaVersion {
		return nil, fmt.Errorf("catalog schema %d is newer than supported (%d)", root.Schema, SchemaVersion)
	}
	c := &Catalog{Schema: root.Schema, Version: root.Version, Items: map[string]*Item{}, Files: fsys}

	cats := map[string]Category{}
	if err := eachYAML(fsys, "categories", func(name string) error {
		var cat Category
		if err := decode(fsys, name, &cat); err != nil {
			return err
		}
		if _, dup := cats[cat.ID]; dup {
			return fmt.Errorf("%s: duplicate category %q", name, cat.ID)
		}
		cats[cat.ID] = cat
		return nil
	}); err != nil {
		return nil, err
	}
	for i, id := range root.Categories {
		cat, ok := cats[id]
		if !ok {
			return nil, fmt.Errorf("catalog.yaml: unknown category %q", id)
		}
		cat.Order = i
		c.Categories = append(c.Categories, cat)
	}

	if err := eachYAML(fsys, "items", func(name string) error {
		it := &Item{}
		if err := decode(fsys, name, it); err != nil {
			return err
		}
		if _, dup := c.Items[it.ID]; dup {
			return fmt.Errorf("%s: duplicate item %q", name, it.ID)
		}
		if it.Shell.InitOrder == 0 {
			it.Shell.InitOrder = DefaultInitOrder
		}
		c.Items[it.ID] = it
		return nil
	}); err != nil {
		return nil, err
	}

	if err := eachYAML(fsys, "profiles", func(name string) error {
		var p Profile
		if err := decode(fsys, name, &p); err != nil {
			return err
		}
		c.Profiles = append(c.Profiles, p)
		return nil
	}); err != nil {
		return nil, err
	}
	return c, nil
}

func decode(fsys fs.FS, name string, v any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// eachYAML chama fn para cada .yaml de dir, em ordem alfabética; dir ausente é ignorado.
func eachYAML(fsys fs.FS, dir string, fn func(name string) error) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".yaml" {
			continue
		}
		if err := fn(path.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
