package plan

import (
	"errors"
	"strings"
	"testing"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
)

var linux = platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "ubuntu", PkgManager: "apt", Shell: "zsh", Home: "/home/dev"}

func mk(id, category string, deps ...string) *catalog.Item {
	return &catalog.Item{
		ID: id, Category: category, Name: i18n.Text{i18n.En: id}, Platforms: []platform.OS{platform.Linux, platform.MacOS},
		DependsOn: deps, Install: []catalog.Method{{Backend: "mise", Package: id}},
	}
}

func testCatalog() *catalog.Catalog {
	c := &catalog.Catalog{
		Categories: []catalog.Category{{ID: "cli"}, {ID: "php-local", Exclusive: true}, {ID: "languages"}},
		Items:      map[string]*catalog.Item{},
	}
	for _, it := range []*catalog.Item{
		mk("fzf", "cli"), mk("bat", "cli"), mk("mise", "cli"), mk("go", "languages", "mise"), mk("node", "languages", "mise"),
		mk("herdr-plugins", "cli", "go"), mk("lerd", "php-local"), mk("flock", "php-local"), mk("php", "languages"),
		mk("zsh-syntax-highlighting", "cli"), mk("zsh", "cli"), mk("build-tools", "cli"),
	} {
		c.Items[it.ID] = it
	}
	c.Items["php"].ProvidedBy = []string{"lerd", "flock"}
	c.Items["node"].Install[0].Version = "22"
	c.Items["zsh-syntax-highlighting"].ZshOnly = true
	c.Items["build-tools"].Install = []catalog.Method{{Backend: "system", Package: "build-essential"}}
	c.Items["bat"].Configs = []catalog.ConfigSpec{{ID: "bat-config", Default: true, Files: []catalog.ConfigFile{{Template: "configs/bat/config", Target: "{{.XDG_CONFIG}}/bat/config"}}}}
	c.Profiles = []catalog.Profile{
		{ID: "base", Items: []string{"fzf", "bat"}},
		{ID: "laravel", Items: []string{"fzf", "php"}, Exclusive: map[string]string{"php-local": "flock"}},
	}
	return c
}

func absent(c *catalog.Catalog) map[string]detect.ItemStatus {
	st := map[string]detect.ItemStatus{}
	for id := range c.Items {
		st[id] = detect.ItemStatus{ID: id, Status: detect.StatusAbsent}
	}
	return st
}

func ids(p *Plan) []string {
	var out []string
	for _, s := range p.Steps {
		out = append(out, s.ID)
	}
	return out
}

func has(p *Plan, id string) bool {
	for _, s := range p.Steps {
		if s.ID == id {
			return true
		}
	}
	return false
}

func step(t *testing.T, p *Plan, id string) Step {
	t.Helper()
	for _, s := range p.Steps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("step %s not in %v", id, ids(p))
	return Step{}
}

func TestProfileItemsInstalled(t *testing.T) {
	c := testCatalog()
	p, err := Build(c, linux, absent(c), Selection{Profile: "base"})
	if err != nil || !has(p, "install:fzf") || !has(p, "install:bat") {
		t.Fatalf("%v %v", ids(p), err)
	}
}

func TestAdditiveNeverRemoves(t *testing.T) {
	c := testCatalog()
	st := absent(c)
	st["node"] = detect.ItemStatus{ID: "node", Status: detect.StatusInstalled}
	p, _ := Build(c, linux, st, Selection{Profile: "base"})
	for _, s := range p.Steps {
		if s.ItemID == "node" || s.Kind == KindUninstall {
			t.Fatalf("unexpected step %s", s.ID)
		}
	}
}

func TestRemoveOnlyDeselects(t *testing.T) {
	c := testCatalog()
	st := absent(c)
	st["bat"] = detect.ItemStatus{ID: "bat", Status: detect.StatusInstalled}
	p, _ := Build(c, linux, st, Selection{Profile: "base", Remove: []string{"bat"}})
	for _, s := range p.Steps {
		if s.ItemID == "bat" {
			t.Fatalf("bat must not have steps: %s", s.ID)
		}
	}
}

func TestDependencyPulled(t *testing.T) {
	c := testCatalog()
	p, err := Build(c, linux, absent(c), Selection{Add: []string{"herdr-plugins"}})
	if err != nil {
		t.Fatal(err)
	}
	g := step(t, p, "install:go")
	if !strings.Contains(g.Note, "herdr-plugins") {
		t.Fatalf("note: %q", g.Note)
	}
	h := step(t, p, "install:herdr-plugins")
	if len(h.DependsOn) != 1 || h.DependsOn[0] != "install:go" {
		t.Fatalf("deps: %v", h.DependsOn)
	}
	layers := p.Order()
	pos := map[string]int{}
	for i, l := range layers {
		for _, s := range l {
			pos[s.ID] = i
		}
	}
	if !(pos["install:mise"] < pos["install:go"] && pos["install:go"] < pos["install:herdr-plugins"]) {
		t.Fatalf("order: %v", pos)
	}
}

func TestExclusiveConflict(t *testing.T) {
	c := testCatalog()
	_, err := Build(c, linux, absent(c), Selection{Add: []string{"lerd", "flock"}})
	var pe *PlanError
	if !errors.As(err, &pe) {
		t.Fatalf("want PlanError, got %v", err)
	}
	msg := pe.Error()
	for _, w := range []string{"lerd", "flock", "php-local"} {
		if !strings.Contains(msg, w) {
			t.Fatalf("%q missing %q", msg, w)
		}
	}
}

func TestExclusiveProfileDefaultOverridden(t *testing.T) {
	c := testCatalog()
	p, err := Build(c, linux, absent(c), Selection{Profile: "laravel", Add: []string{"lerd"}})
	if err != nil || !has(p, "install:lerd") || has(p, "install:flock") {
		t.Fatalf("%v %v", ids(p), err)
	}
}

func TestProvidedSkipsPHP(t *testing.T) {
	c := testCatalog()
	st := absent(c)
	st["php"] = detect.ItemStatus{ID: "php", Status: detect.StatusProvided, Reason: "flock"}
	p, _ := Build(c, linux, st, Selection{Profile: "laravel"})
	if has(p, "install:php") || !has(p, "install:flock") {
		t.Fatalf("%v", ids(p))
	}
}

func TestUnsupportedExplicitWarns(t *testing.T) {
	c := testCatalog()
	st := absent(c)
	st["flock"] = detect.ItemStatus{ID: "flock", Status: detect.StatusUnsupported, Reason: "requires ubuntu>=24.04"}
	p, err := Build(c, linux, st, Selection{Add: []string{"flock", "fzf"}})
	if err != nil || has(p, "install:flock") || !has(p, "install:fzf") {
		t.Fatalf("%v %v", ids(p), err)
	}
	if len(p.Skipped) == 0 || p.Skipped[0].ItemID != "flock" || !strings.Contains(strings.Join(p.Warnings, " "), "flock") {
		t.Fatalf("skipped %v warnings %v", p.Skipped, p.Warnings)
	}
}

func TestZshOnlySkippedOnBash(t *testing.T) {
	c := testCatalog()
	bash := linux
	bash.Shell = "bash"
	p, _ := Build(c, bash, absent(c), Selection{Add: []string{"zsh-syntax-highlighting"}})
	if has(p, "install:zsh-syntax-highlighting") || len(p.Skipped) != 1 {
		t.Fatalf("%v %v", ids(p), p.Skipped)
	}
	p, _ = Build(c, bash, absent(c), Selection{Add: []string{"zsh-syntax-highlighting"}, SwitchToZsh: true})
	if !has(p, "install:zsh-syntax-highlighting") || !has(p, "chsh:zsh") {
		t.Fatalf("with zsh switch: %v", ids(p))
	}
}

func TestOrphanSuggested(t *testing.T) {
	c := testCatalog()
	st := absent(c)
	st["node"] = detect.ItemStatus{ID: "node", Status: detect.StatusInstalled}
	st["mise"] = detect.ItemStatus{ID: "mise", Status: detect.StatusInstalled}
	p, _ := Build(c, linux, st, Selection{Uninstall: []string{"node"}})
	if !has(p, "uninstall:node") || has(p, "uninstall:mise") || len(p.Orphans) != 1 || p.Orphans[0] != "mise" {
		t.Fatalf("%v orphans %v", ids(p), p.Orphans)
	}
	st["go"] = detect.ItemStatus{ID: "go", Status: detect.StatusInstalled}
	p, _ = Build(c, linux, st, Selection{Uninstall: []string{"node"}})
	if len(p.Orphans) != 0 {
		t.Fatalf("mise still used by go: %v", p.Orphans)
	}
}

func TestOrderSudoFirst(t *testing.T) {
	c := testCatalog()
	p, _ := Build(c, linux, absent(c), Selection{Add: []string{"fzf", "build-tools"}})
	first := p.Order()[0]
	if !first[0].NeedsSudo || first[0].ItemID != "build-tools" {
		t.Fatalf("first layer: %+v", first)
	}
}

func TestVersionsOverride(t *testing.T) {
	c := testCatalog()
	p, _ := Build(c, linux, absent(c), Selection{Add: []string{"node"}, Versions: map[string]string{"node": "24"}})
	if got := step(t, p, "install:node").Cmds[0].String(); !strings.Contains(got, "node@24") {
		t.Fatalf("cmd %q", got)
	}
}

func TestConfigStepDefaultAndToggle(t *testing.T) {
	c := testCatalog()
	p, _ := Build(c, linux, absent(c), Selection{Profile: "base"})
	cfg := step(t, p, "config:bat-config")
	if cfg.DependsOn[0] != "install:bat" || cfg.Files[0].Target != "{{.XDG_CONFIG}}/bat/config" {
		t.Fatalf("config step %+v", cfg)
	}
	p, _ = Build(c, linux, absent(c), Selection{Profile: "base", Configs: map[string]bool{"bat-config": false}})
	if has(p, "config:bat-config") {
		t.Fatal("config disabled must not have a step")
	}
}

func TestUpdateOnlyOwned(t *testing.T) {
	c := testCatalog()
	st := absent(c)
	st["node"] = detect.ItemStatus{ID: "node", Status: detect.StatusInstalled}
	st["fzf"] = detect.ItemStatus{ID: "fzf", Status: detect.StatusExternal}
	p, _ := Build(c, linux, st, Selection{UpdateAll: true})
	if !has(p, "update:node") || has(p, "update:fzf") {
		t.Fatalf("%v", ids(p))
	}
	if got := step(t, p, "update:node").Cmds[0].String(); got != "mise upgrade node" {
		t.Fatalf("cmd %q", got)
	}
}

func TestUnknownProfile(t *testing.T) {
	c := testCatalog()
	if _, err := Build(c, linux, absent(c), Selection{Profile: "nope"}); err == nil {
		t.Fatal("want error")
	}
}

func TestPlanJSONRoundTrip(t *testing.T) {
	c := testCatalog()
	p, _ := Build(c, linux, absent(c), Selection{Profile: "base"})
	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil || len(got.Steps) != len(p.Steps) || got.Steps[0].Cmds[0].String() != p.Steps[0].Cmds[0].String() {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
}
