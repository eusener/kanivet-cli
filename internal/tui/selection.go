// Package tui implementa a interface de terminal do kanivet-cli.
// Este arquivo guarda o modelo puro de seleção, sem dependência do Bubble Tea.
package tui

import (
	"slices"
	"sort"

	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
)

type Mode int

const (
	ModeWizard Mode = iota
	ModeDashboard
)

// Selection guarda o que o usuário marcou e o converte em plan.Selection.
type Selection struct {
	c         *catalog.Catalog
	p         platform.Platform
	st        map[string]detect.ItemStatus
	mode      Mode
	marked    map[string]bool
	profile   string
	versions  map[string]string
	cfgs      map[string]bool
	cfgDef    map[string]bool
	updates   map[string]bool
	updateAll bool
}

func NewSelection(c *catalog.Catalog, p platform.Platform, st map[string]detect.ItemStatus, mode Mode) *Selection {
	s := &Selection{
		c: c, p: p, st: st, mode: mode,
		marked: map[string]bool{}, versions: map[string]string{},
		cfgs: map[string]bool{}, cfgDef: map[string]bool{}, updates: map[string]bool{},
	}
	for _, it := range c.Items {
		for _, cf := range it.Configs {
			s.cfgDef[cf.ID] = cf.Default
		}
	}
	if mode == ModeDashboard {
		for id, x := range st {
			if owned(x.Status) || external(x.Status) {
				s.marked[id] = true
			}
		}
	}
	return s
}

func owned(s detect.Status) bool { return s == detect.StatusInstalled || s == detect.StatusOutdated }
func external(s detect.Status) bool {
	return s == detect.StatusExternal || s == detect.StatusExternalOutdated
}
func present(s detect.Status) bool { return owned(s) || external(s) || s == detect.StatusProvided }

func (s *Selection) status(id string) detect.ItemStatus {
	if x, ok := s.st[id]; ok {
		return x
	}
	return detect.ItemStatus{ID: id, Status: detect.StatusAbsent}
}

// profileSet devolve os itens do perfil escolhido, com as exclusivas resolvidas para este sistema.
func (s *Selection) profileSet() map[string]bool {
	out := map[string]bool{}
	for _, pr := range s.c.Profiles {
		if pr.ID != s.profile {
			continue
		}
		for _, id := range pr.Items {
			out[id] = true
		}
		for _, id := range pr.Exclusive {
			out[id] = true
		}
		for _, id := range pr.ExclusiveByOS[s.p.OS] {
			out[id] = true
		}
	}
	return out
}

// ApplyProfile marca (de forma aditiva) os itens do perfil e as suas exclusivas.
func (s *Selection) ApplyProfile(id string) {
	s.profile = id
	for _, pr := range s.c.Profiles {
		if pr.ID != id {
			continue
		}
		for _, it := range pr.Items {
			s.mark(it)
		}
		excl := map[string]string{}
		for cat, it := range pr.Exclusive {
			excl[cat] = it
		}
		for cat, it := range pr.ExclusiveByOS[s.p.OS] {
			excl[cat] = it
		}
		for _, it := range excl {
			s.mark(it)
		}
	}
}

// mark marca um item selecionável, desmarcando os da mesma categoria exclusiva.
func (s *Selection) mark(id string) {
	it := s.c.Items[id]
	if it == nil {
		return
	}
	if ok, _ := s.Selectable(id); !ok {
		return
	}
	for _, cat := range s.c.Categories {
		if cat.ID == it.Category && cat.Exclusive {
			for oid, o := range s.c.Items {
				if oid != id && o.Category == cat.ID {
					delete(s.marked, oid)
				}
			}
		}
	}
	s.marked[id] = true
}

func (s *Selection) Marked(id string) bool { return s.marked[id] }

// Selectable é falso para itens sem suporte ou fornecidos por outro item, com o motivo.
func (s *Selection) Selectable(id string) (bool, string) {
	if s.c.Items[id] == nil {
		return false, ""
	}
	x := s.status(id)
	if x.Status == detect.StatusUnsupported || x.Status == detect.StatusProvided {
		return false, x.Reason
	}
	return true, ""
}

// Toggle alterna a marca. Quando não muda, note traz o motivo.
func (s *Selection) Toggle(id string) (bool, string) {
	if ok, why := s.Selectable(id); !ok {
		return false, why
	}
	x := s.status(id)
	if s.marked[id] {
		if s.mode == ModeDashboard && external(x.Status) {
			return false, i18n.T("tui.external_untouched")
		}
		delete(s.marked, id)
		return true, ""
	}
	s.mark(id)
	return true, ""
}

func (s *Selection) ConfigOn(cfgID string) bool {
	if v, ok := s.cfgs[cfgID]; ok {
		return v
	}
	return s.cfgDef[cfgID]
}

func (s *Selection) ToggleConfig(cfgID string) { s.cfgs[cfgID] = !s.ConfigOn(cfgID) }

// lines devolve as linhas de versão do método escolhido (a primeira é a padrão).
func (s *Selection) lines(itemID string) []string {
	it := s.c.Items[itemID]
	if it == nil {
		return nil
	}
	m, _, err := backend.Choose(it, s.p)
	if err != nil {
		return nil
	}
	if len(m.Versions) > 0 {
		return m.Versions
	}
	if m.Version != "" {
		return []string{m.Version}
	}
	return nil
}

func (s *Selection) Version(itemID string) string {
	if v, ok := s.versions[itemID]; ok {
		return v
	}
	if l := s.lines(itemID); len(l) > 0 {
		return l[0]
	}
	return ""
}

func (s *Selection) CycleVersion(itemID string, delta int) {
	l := s.lines(itemID)
	if len(l) < 2 {
		return
	}
	i := slices.Index(l, s.Version(itemID))
	if i < 0 {
		i = 0
	}
	n := len(l)
	i = ((i+delta)%n + n) % n
	if i == 0 {
		delete(s.versions, itemID)
		return
	}
	s.versions[itemID] = l[i]
}

func (s *Selection) MarkUpdate(id string) { s.updates[id] = true }
func (s *Selection) ToggleUpdateAll()     { s.updateAll = !s.updateAll }

// Pending conta o que será instalado, atualizado e removido.
func (s *Selection) Pending() (install, update, remove int) {
	install = len(s.toInstall())
	upd := map[string]bool{}
	for id := range s.updates {
		upd[id] = true
	}
	if s.updateAll {
		for id, x := range s.st {
			if x.Status == detect.StatusOutdated {
				upd[id] = true
			}
		}
	}
	update = len(upd)
	if s.mode == ModeDashboard {
		remove = len(s.ToPlan().Uninstall)
	}
	return
}

// toInstall lista os marcados que ainda não estão presentes.
func (s *Selection) toInstall() []string {
	var out []string
	for id := range s.marked {
		if !present(s.status(id).Status) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Selection) ToPlan() plan.Selection {
	var out plan.Selection
	switch s.mode {
	case ModeWizard:
		out.Profile = s.profile
		for id := range s.marked {
			out.Add = append(out.Add, id)
		}
		sort.Strings(out.Add)
		// Remove só desmarca itens do perfil; nunca desinstala.
		for id := range s.profileSet() {
			if !s.marked[id] && s.c.Items[id] != nil {
				out.Remove = append(out.Remove, id)
			}
		}
		sort.Strings(out.Remove)
	case ModeDashboard:
		out.Add = s.toInstall()
		for id, x := range s.st {
			if owned(x.Status) && !s.marked[id] {
				out.Uninstall = append(out.Uninstall, id)
			}
		}
		sort.Strings(out.Uninstall)
	}
	for id := range s.updates {
		out.Update = append(out.Update, id)
	}
	sort.Strings(out.Update)
	out.UpdateAll = s.updateAll
	for id, v := range s.versions {
		if out.Versions == nil {
			out.Versions = map[string]string{}
		}
		out.Versions[id] = v
	}
	for id, v := range s.cfgs {
		if v != s.cfgDef[id] {
			if out.Configs == nil {
				out.Configs = map[string]bool{}
			}
			out.Configs[id] = v
		}
	}
	return out
}

// ConfigsOn conta as configs ligadas dos itens marcados (os que serão instalados ou já estão presentes).
func (s *Selection) ConfigsOn() int {
	n := 0
	for id := range s.marked {
		if it := s.c.Items[id]; it != nil {
			for _, cf := range it.Configs {
				if s.ConfigOn(cf.ID) {
					n++
				}
			}
		}
	}
	return n
}
