package catalog

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/eusener/kanivet-cli/internal/i18n"
)

type LintError struct{ Item, Field, Msg string }

func (e LintError) Error() string { return fmt.Sprintf("%s: %s: %s", e.Item, e.Field, e.Msg) }

// Lint valida o catálogo; backends é a lista de backends conhecidos.
func Lint(c *Catalog, backends []string) []error {
	var errs []error
	add := func(item, field, format string, args ...any) {
		errs = append(errs, LintError{item, field, fmt.Sprintf(format, args...)})
	}
	cats := map[string]Category{}
	for _, cat := range c.Categories {
		cats[cat.ID] = cat
	}
	known := map[string]bool{}
	for _, b := range backends {
		known[b] = true
	}

	for _, id := range sortedIDs(c) {
		it := c.Items[id]
		lintText(it.ID, "name", it.Name, add)
		lintText(it.ID, "description", it.Description, add)
		if _, ok := cats[it.Category]; !ok {
			add(it.ID, "category", "unknown category %q", it.Category)
		}
		if len(it.Platforms) == 0 {
			add(it.ID, "platforms", "at least one platform is required")
		}
		for _, ref := range it.DependsOn {
			if c.Items[ref] == nil {
				add(it.ID, "depends_on", "unknown item %q", ref)
			}
		}
		for _, ref := range it.ProvidedBy {
			if c.Items[ref] == nil {
				add(it.ID, "provided_by", "unknown item %q", ref)
			}
		}
		for i, m := range it.Install {
			field := fmt.Sprintf("install[%d]", i)
			if !known[m.Backend] {
				add(it.ID, field, "unknown backend %q", m.Backend)
			}
			if m.Backend == "script" && m.Inline == "" && !strings.HasPrefix(m.URL, "https://") {
				add(it.ID, field, "url must be https:// (got %q)", m.URL)
			}
		}
		for _, cfg := range it.Configs {
			for _, f := range cfg.Files {
				if f.Dir {
					continue
				}
				if _, err := fs.Stat(c.Files, f.Template); err != nil {
					add(it.ID, "configs."+cfg.ID, "template %s not found", f.Template)
				}
			}
		}
	}
	for _, cyc := range findCycles(c) {
		add(cyc[0], "depends_on", "cycle: %s", strings.Join(cyc, " -> "))
	}
	for _, p := range c.Profiles {
		for _, ref := range p.Items {
			if c.Items[ref] == nil {
				add(p.ID, "items", "unknown item %q", ref)
			}
		}
		excl := []map[string]string{p.Exclusive}
		for _, m := range p.ExclusiveByOS {
			excl = append(excl, m)
		}
		for _, m := range excl {
			lintProfileExclusive(p.ID, m, cats, c, add)
		}
	}
	return errs
}

func lintProfileExclusive(profile string, m map[string]string, cats map[string]Category, c *Catalog, add func(string, string, string, ...any)) {
	for cat, ref := range m {
		if !cats[cat].Exclusive {
			add(profile, "exclusive", "category %q is not exclusive", cat)
		}
		if it := c.Items[ref]; it == nil || it.Category != cat {
			add(profile, "exclusive", "item %q is not in category %q", ref, cat)
		}
	}
}

func lintText(item, field string, t i18n.Text, add func(string, string, string, ...any)) {
	if t[i18n.En] == "" {
		add(item, field, "missing en")
	}
}

func sortedIDs(c *Catalog) []string {
	ids := make([]string, 0, len(c.Items))
	for id := range c.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// findCycles faz DFS branco/cinza/preto sobre DependsOn e devolve cada ciclo encontrado.
func findCycles(c *Catalog) [][]string {
	const (
		white = iota
		gray
		black
	)
	color := map[string]int{}
	var stack []string
	var cycles [][]string
	var visit func(id string)
	visit = func(id string) {
		color[id] = gray
		stack = append(stack, id)
		if it := c.Items[id]; it != nil {
			for _, dep := range it.DependsOn {
				switch color[dep] {
				case white:
					if c.Items[dep] != nil {
						visit(dep)
					}
				case gray:
					for i, s := range stack {
						if s == dep {
							cyc := append(append([]string{}, stack[i:]...), dep)
							cycles = append(cycles, cyc)
						}
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
	}
	for _, id := range sortedIDs(c) {
		if color[id] == white {
			visit(id)
		}
	}
	return cycles
}
