// Package plan transforma a seleção do usuário e o estado da máquina num plano de passos.
package plan

import (
	"encoding/json"

	"sort"
	"strings"

	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

type Selection struct {
	Profile   string
	Add       []string
	Remove    []string
	Uninstall []string
	Versions  map[string]string // item → linha ("node" → "24")
	Configs   map[string]bool   // configID → ligada; ausente = ConfigSpec.Default
	Update    []string
	UpdateAll bool
	// SwitchToZsh instala o zsh e o define como shell padrão (chsh).
	SwitchToZsh bool
}

type StepKind string

const (
	KindInstall   StepKind = "install"
	KindUpdate    StepKind = "update"
	KindUninstall StepKind = "uninstall"
	KindConfig    StepKind = "config"
	KindShell     StepKind = "shell"
	KindChsh      StepKind = "chsh"
)

// ShellStepID é o passo único que regenera o shell init.
const ShellStepID = "shell:all"

type FileChange struct {
	Target   string `json:"target"`
	Template string `json:"template,omitempty"`
	ItemID   string `json:"item"`
	Remove   bool   `json:"remove,omitempty"`
	Dir      bool   `json:"dir,omitempty"`
}

type Step struct {
	ID        string       `json:"id"`
	ItemID    string       `json:"item,omitempty"`
	Kind      StepKind     `json:"kind"`
	Cmds      []runner.Cmd `json:"cmds,omitempty"`
	Files     []FileChange `json:"files,omitempty"`
	NeedsSudo bool         `json:"needs_sudo,omitempty"`
	DependsOn []string     `json:"depends_on,omitempty"`
	Note      string       `json:"note,omitempty"`
	Backend   string       `json:"backend,omitempty"`
	Package   string       `json:"package,omitempty"`
}

type Skip struct {
	ItemID string `json:"item"`
	Reason string `json:"reason"`
}

type Plan struct {
	Steps    []Step   `json:"steps"`
	Skipped  []Skip   `json:"skipped,omitempty"`
	Orphans  []string `json:"orphans,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type PlanError struct{ Msgs []string }

func (e *PlanError) Error() string { return strings.Join(e.Msgs, "; ") }

func (p *Plan) MarshalJSON() ([]byte, error) {
	type alias Plan
	return json.Marshal((*alias)(p))
}

func Decode(data []byte) (*Plan, error) {
	p := &Plan{}
	type alias Plan
	if err := json.Unmarshal(data, (*alias)(p)); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Plan) Step(id string) (Step, bool) {
	for _, s := range p.Steps {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}

type builder struct {
	c        *catalog.Catalog
	p        platform.Platform
	st       map[string]detect.ItemStatus
	sel      Selection
	plan     *Plan
	explicit map[string]bool
	notes    map[string]string
	inPlan   map[string]bool // passos de install criados, por item
	skipped  map[string]bool
}

// Build monta o plano. Seleção é aditiva: install nunca gera passos de remoção.
func Build(c *catalog.Catalog, p platform.Platform, st map[string]detect.ItemStatus, sel Selection) (*Plan, error) {
	b := &builder{c: c, p: p, st: st, sel: sel, plan: &Plan{},
		explicit: map[string]bool{}, notes: map[string]string{}, inPlan: map[string]bool{}, skipped: map[string]bool{}}
	desired, err := b.desired()
	if err != nil {
		return nil, err
	}
	for _, id := range b.closure(desired) {
		b.addInstall(id)
	}
	b.addConfigsAndShell(desired)
	b.addUpdates()
	b.addUninstalls()
	b.addChsh()
	return b.plan, nil
}

func (b *builder) status(id string) detect.Status {
	if s, ok := b.st[id]; ok {
		return s.Status
	}
	return detect.StatusAbsent
}

// desired aplica perfil + Add − Remove e resolve as categorias exclusivas.
func (b *builder) desired() ([]string, error) {
	var base []string
	exclusiveDefaults := map[string]string{}
	if b.sel.Profile != "" {
		prof := b.profile(b.sel.Profile)
		if prof == nil {
			return nil, &PlanError{[]string{i18n.T("err.unknown_profile", b.sel.Profile)}}
		}
		base = append(base, prof.Items...)
		for cat, id := range prof.Exclusive {
			exclusiveDefaults[cat] = id
		}
		for cat, id := range prof.ExclusiveByOS[b.p.OS] {
			exclusiveDefaults[cat] = id
		}
	}
	var msgs []string
	for _, id := range b.sel.Add {
		if b.c.Items[id] == nil {
			msgs = append(msgs, i18n.T("err.unknown_item", id))
			continue
		}
		b.explicit[id] = true
	}
	if b.sel.SwitchToZsh && b.c.Items["zsh"] != nil {
		base = append(base, "zsh")
	}
	explicitCats := map[string]bool{}
	for id := range b.explicit {
		explicitCats[b.c.Items[id].Category] = true
	}
	cats := make([]string, 0, len(exclusiveDefaults))
	for cat := range exclusiveDefaults {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	for _, cat := range cats {
		if !explicitCats[cat] {
			base = append(base, exclusiveDefaults[cat])
		}
	}
	all := append(base, b.sel.Add...)
	removed := map[string]bool{}
	for _, id := range b.sel.Remove {
		removed[id] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, id := range all {
		if seen[id] || removed[id] || b.c.Items[id] == nil {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}

	exclusive := map[string]bool{}
	for _, cat := range b.c.Categories {
		exclusive[cat.ID] = cat.Exclusive
	}
	byCat := map[string][]string{}
	for _, id := range out {
		cat := b.c.Items[id].Category
		if exclusive[cat] {
			byCat[cat] = append(byCat[cat], id)
		}
	}
	for _, cat := range sortedKeys(byCat) {
		if ids := byCat[cat]; len(ids) > 1 {
			msgs = append(msgs, i18n.T("err.exclusive", strings.Join(ids, ", "), cat))
		}
	}
	if len(msgs) > 0 {
		return nil, &PlanError{msgs}
	}
	return out, nil
}

func (b *builder) profile(id string) *catalog.Profile {
	for i := range b.c.Profiles {
		if b.c.Profiles[i].ID == id {
			return &b.c.Profiles[i]
		}
	}
	return nil
}

// closure devolve os itens com suas dependências, dependências primeiro.
func (b *builder) closure(desired []string) []string {
	var out []string
	visited := map[string]bool{}
	var visit func(id, parent string)
	visit = func(id, parent string) {
		if visited[id] {
			return
		}
		visited[id] = true
		if parent != "" {
			b.notes[id] = i18n.T("note.dependency_of", parent)
		}
		if b.willInstall(id) {
			for _, dep := range b.depsOf(id) {
				visit(dep, id)
			}
		}
		out = append(out, id)
	}
	for _, id := range desired {
		visit(id, "")
	}
	return out
}

// implicitDeps: o item que fornece cada backend (fzf via mise precisa do mise instalado).
var implicitDeps = map[string]string{"mise": "mise", "brew": "homebrew"}

// depsOf devolve as dependências declaradas mais a do backend escolhido para esta plataforma.
func (b *builder) depsOf(id string) []string {
	it := b.c.Items[id]
	var deps []string
	for _, d := range it.DependsOn {
		if b.c.Items[d] != nil {
			deps = append(deps, d)
		}
	}
	if m, _, err := backend.Choose(it, b.p); err == nil {
		if d, ok := implicitDeps[m.Backend]; ok && d != id && b.c.Items[d] != nil && !contains(deps, d) {
			deps = append(deps, d)
		}
	}
	return deps
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (b *builder) configOn(cfg catalog.ConfigSpec) bool {
	on, set := b.sel.Configs[cfg.ID]
	if !set {
		on = cfg.Default
	}
	return on
}

func isDirConfig(cfg catalog.ConfigSpec) bool {
	for _, f := range cfg.Files {
		if !f.Dir {
			return false
		}
	}
	return len(cfg.Files) > 0
}

// willInstall diz se o item vai ganhar um passo de instalação (e portanto precisa das dependências).
func (b *builder) willInstall(id string) bool {
	it := b.c.Items[id]
	switch b.status(id) {
	case detect.StatusAbsent, detect.StatusExternalOutdated:
	default:
		return false
	}
	if it.ZshOnly && b.p.Shell != "zsh" && !b.sel.SwitchToZsh {
		return false
	}
	_, _, err := backend.Choose(it, b.p)
	return err == nil
}

func (b *builder) skip(id, reason string) {
	b.skipped[id] = true
	b.plan.Skipped = append(b.plan.Skipped, Skip{ItemID: id, Reason: reason})
	if b.explicit[id] {
		b.plan.Warnings = append(b.plan.Warnings, i18n.T("warn.skipped", id, reason))
	}
}

func (b *builder) addInstall(id string) {
	it := b.c.Items[id]
	s := b.st[id]
	switch {
	case s.Status == detect.StatusUnsupported:
		b.skip(id, s.Reason)
		return
	case it.ZshOnly && b.p.Shell != "zsh" && !b.sel.SwitchToZsh:
		b.skip(id, i18n.T("reason.zsh_only"))
		return
	case s.Status == detect.StatusProvided:
		b.skip(id, i18n.T("reason.provided_by", s.Reason))
		return
	case s.Status == detect.StatusExternalOutdated:
		b.plan.Warnings = append(b.plan.Warnings, i18n.T("warn.external_outdated", id, s.Version, it.MinVersion))
	case s.Status != detect.StatusAbsent:
		return
	}
	for _, dep := range b.depsOf(id) {
		if b.skipped[dep] {
			b.skip(id, i18n.T("reason.dependency_skipped", dep))
			return
		}
	}
	m, be, err := backend.Choose(it, b.p)
	if err != nil {
		b.skip(id, err.Error())
		return
	}
	cmds := be.Install(m, b.sel.Versions[id], b.p)
	for _, post := range it.PostInstall {
		cmds = append(cmds, runner.Cmd{Shell: post})
	}
	st := Step{ID: "install:" + id, ItemID: id, Kind: KindInstall, Cmds: cmds, Note: b.notes[id],
		NeedsSudo: it.NeedsSudo || m.NeedsSudo || anySudo(cmds), Backend: be.Name(), Package: m.Package}
	for _, dep := range b.depsOf(id) {
		if b.inPlan[dep] {
			st.DependsOn = append(st.DependsOn, "install:"+dep)
		}
	}
	// Diretórios que a instalação recria (ex.: ~/.config/nvim) vão para o backup antes.
	for _, cfg := range it.Configs {
		if !isDirConfig(cfg) || !b.configOn(cfg) {
			continue
		}
		dirStep := Step{ID: "config:" + cfg.ID, ItemID: id, Kind: KindConfig}
		for _, f := range cfg.Files {
			dirStep.Files = append(dirStep.Files, FileChange{Target: f.Target, ItemID: id, Dir: true})
		}
		b.plan.Steps = append(b.plan.Steps, dirStep)
		st.DependsOn = append(st.DependsOn, dirStep.ID)
	}
	b.inPlan[id] = true
	b.plan.Steps = append(b.plan.Steps, st)
}

func (b *builder) addConfigsAndShell(desired []string) {
	needsShell := false
	var installs []string
	for _, id := range b.closure(desired) {
		if b.skipped[id] {
			continue
		}
		it := b.c.Items[id]
		if b.inPlan[id] {
			installs = append(installs, "install:"+id)
		}
		if len(it.Shell.Aliases) > 0 || len(it.Shell.Init) > 0 || len(it.Shell.Env) > 0 || len(it.Env) > 0 {
			needsShell = true
		}
		for _, cfg := range it.Configs {
			if !b.configOn(cfg) || isDirConfig(cfg) {
				continue
			}
			st := Step{ID: "config:" + cfg.ID, ItemID: id, Kind: KindConfig}
			for _, f := range cfg.Files {
				st.Files = append(st.Files, FileChange{Target: f.Target, Template: f.Template, ItemID: id})
			}
			if b.inPlan[id] {
				st.DependsOn = []string{"install:" + id}
			}
			b.plan.Steps = append(b.plan.Steps, st)
		}
	}
	if needsShell {
		b.addShellStep(installs)
	}
}

func (b *builder) addShellStep(deps []string) {
	for i, s := range b.plan.Steps {
		if s.ID == ShellStepID {
			b.plan.Steps[i].DependsOn = append(b.plan.Steps[i].DependsOn, deps...)
			return
		}
	}
	b.plan.Steps = append(b.plan.Steps, Step{ID: ShellStepID, Kind: KindShell, DependsOn: deps})
}

func (b *builder) owned(id string) bool {
	s := b.status(id)
	return s == detect.StatusInstalled || s == detect.StatusOutdated
}

func (b *builder) addUpdates() {
	ids := b.sel.Update
	if b.sel.UpdateAll {
		ids = nil
		for _, id := range sortedKeys(b.c.Items) {
			if b.owned(id) {
				ids = append(ids, id)
			}
		}
	}
	for _, id := range ids {
		it := b.c.Items[id]
		if it == nil || !b.owned(id) {
			if it != nil {
				b.plan.Warnings = append(b.plan.Warnings, i18n.T("warn.not_owned", id))
			}
			continue
		}
		m, be, err := backend.Choose(it, b.p)
		if err != nil {
			continue
		}
		cmds := be.Update(m, b.p)
		if len(cmds) == 0 {
			continue
		}
		b.plan.Steps = append(b.plan.Steps, Step{ID: "update:" + id, ItemID: id, Kind: KindUpdate, Cmds: cmds,
			NeedsSudo: anySudo(cmds), Backend: be.Name(), Package: m.Package})
	}
}

func (b *builder) addUninstalls() {
	if len(b.sel.Uninstall) == 0 {
		return
	}
	removing := map[string]bool{}
	var steps []string
	for _, id := range b.sel.Uninstall {
		it := b.c.Items[id]
		if it == nil {
			b.plan.Warnings = append(b.plan.Warnings, i18n.T("err.unknown_item", id))
			continue
		}
		if !b.owned(id) {
			b.plan.Warnings = append(b.plan.Warnings, i18n.T("warn.not_owned", id))
			continue
		}
		m, be, err := backend.Choose(it, b.p)
		if err != nil {
			continue
		}
		var dirFiles []FileChange
		for _, cfg := range it.Configs {
			if isDirConfig(cfg) {
				for _, f := range cfg.Files {
					dirFiles = append(dirFiles, FileChange{Target: f.Target, ItemID: id, Dir: true, Remove: true})
				}
			}
		}
		cmds := be.Uninstall(m, b.p)
		if len(dirFiles) > 0 {
			cmds = nil // o diretório vai para o backup e o original volta; nada de rm -rf
		}
		if len(cmds) == 0 && len(dirFiles) == 0 {
			b.plan.Warnings = append(b.plan.Warnings, i18n.T("warn.manual_uninstall", id))
			continue
		}
		st := Step{ID: "uninstall:" + id, ItemID: id, Kind: KindUninstall, Cmds: cmds, NeedsSudo: anySudo(cmds),
			Backend: be.Name(), Package: m.Package}
		st.Files = append(st.Files, dirFiles...)
		for _, cfg := range it.Configs {
			if isDirConfig(cfg) {
				continue
			}
			for _, f := range cfg.Files {
				st.Files = append(st.Files, FileChange{Target: f.Target, ItemID: id, Remove: true})
			}
		}
		removing[id] = true
		steps = append(steps, st.ID)
		b.plan.Steps = append(b.plan.Steps, st)
	}
	if len(steps) > 0 {
		b.addShellStep(steps)
	}
	// Órfãos: dependências nossas que nenhum item restante presente usa.
	candidates := map[string]bool{}
	for id := range removing {
		for _, dep := range b.c.Items[id].DependsOn {
			if b.owned(dep) && !removing[dep] {
				candidates[dep] = true
			}
		}
	}
	for _, dep := range sortedKeys(candidates) {
		used := false
		for id, it := range b.c.Items {
			if removing[id] || !b.status(id).Present() {
				continue
			}
			for _, d := range it.DependsOn {
				if d == dep {
					used = true
				}
			}
		}
		if !used {
			b.plan.Orphans = append(b.plan.Orphans, dep)
		}
	}
}

func (b *builder) addChsh() {
	if !b.sel.SwitchToZsh || b.p.Shell == "zsh" {
		return
	}
	st := Step{ID: "chsh:zsh", ItemID: "zsh", Kind: KindChsh,
		Cmds: []runner.Cmd{{Shell: `chsh -s "$(command -v zsh)"`, Stdin: true, Display: "chsh -s zsh"}}}
	if b.inPlan["zsh"] {
		st.DependsOn = []string{"install:zsh"}
	}
	b.plan.Steps = append(b.plan.Steps, st)
}

// Order agrupa os passos em camadas topológicas; dentro de cada camada, sudo vem primeiro.
func (p *Plan) Order() [][]Step {
	byID := map[string]Step{}
	indeg := map[string]int{}
	children := map[string][]string{}
	for _, s := range p.Steps {
		byID[s.ID] = s
	}
	for _, s := range p.Steps {
		for _, d := range s.DependsOn {
			if _, ok := byID[d]; ok {
				indeg[s.ID]++
				children[d] = append(children[d], s.ID)
			}
		}
	}
	var layers [][]Step
	var current []string
	for _, s := range p.Steps {
		if indeg[s.ID] == 0 {
			current = append(current, s.ID)
		}
	}
	for len(current) > 0 {
		layer := make([]Step, 0, len(current))
		var next []string
		for _, id := range current {
			layer = append(layer, byID[id])
			for _, ch := range children[id] {
				if indeg[ch]--; indeg[ch] == 0 {
					next = append(next, ch)
				}
			}
		}
		sort.SliceStable(layer, func(i, j int) bool {
			if layer[i].NeedsSudo != layer[j].NeedsSudo {
				return layer[i].NeedsSudo
			}
			return layer[i].ID < layer[j].ID
		})
		layers = append(layers, layer)
		current = next
	}
	return layers
}

func anySudo(cmds []runner.Cmd) bool {
	for _, c := range cmds {
		if c.Sudo {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
