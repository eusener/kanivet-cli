package catalog

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
)

var testBackends = []string{"brew", "mise", "script"}

func validItem(id string) *Item {
	return &Item{
		ID: id, Category: "shell", Name: i18n.Text{i18n.En: id}, Description: i18n.Text{i18n.En: id},
		Platforms: []platform.OS{platform.Linux},
		Install:   []Method{{Backend: "mise", Package: id}},
	}
}

func baseCatalog(items ...*Item) *Catalog {
	c := &Catalog{
		Categories: []Category{{ID: "shell"}, {ID: "php-local", Exclusive: true}},
		Items:      map[string]*Item{},
		Files:      fstest.MapFS{"configs/a.conf": {Data: []byte("x")}},
	}
	for _, it := range items {
		c.Items[it.ID] = it
	}
	return c
}

func wantLint(t *testing.T, c *Catalog, substr string) {
	t.Helper()
	for _, e := range Lint(c, testBackends) {
		if strings.Contains(e.Error(), substr) {
			return
		}
	}
	t.Fatalf("want lint error containing %q, got %v", substr, Lint(c, testBackends))
}

func TestLintMissingEnglish(t *testing.T) {
	it := validItem("a")
	it.Description = i18n.Text{i18n.PtBR: "só pt"}
	wantLint(t, baseCatalog(it), "a: description: missing en")
}

func TestLintUnknownCategory(t *testing.T) {
	it := validItem("a")
	it.Category = "nope"
	wantLint(t, baseCatalog(it), "a: category")
}

func TestLintUnknownReferences(t *testing.T) {
	it := validItem("a")
	it.DependsOn = []string{"ghost"}
	it.ProvidedBy = []string{"phantom"}
	c := baseCatalog(it)
	wantLint(t, c, "ghost")
	wantLint(t, c, "phantom")
	c.Profiles = []Profile{{ID: "p", Items: []string{"spirit"}}}
	wantLint(t, c, "spirit")
}

func TestLintCycle(t *testing.T) {
	a, b := validItem("a"), validItem("b")
	a.DependsOn, b.DependsOn = []string{"b"}, []string{"a"}
	wantLint(t, baseCatalog(a, b), "cycle")
}

func TestLintUnknownBackend(t *testing.T) {
	it := validItem("a")
	it.Install[0].Backend = "apt-magic"
	wantLint(t, baseCatalog(it), "apt-magic")
}

func TestLintMissingTemplate(t *testing.T) {
	it := validItem("a")
	it.Configs = []ConfigSpec{{ID: "c", Description: i18n.Text{i18n.En: "c"}, Files: []ConfigFile{{Template: "configs/missing.conf", Target: "x"}}}}
	wantLint(t, baseCatalog(it), "configs/missing.conf")
}

func TestLintEmptyPlatforms(t *testing.T) {
	it := validItem("a")
	it.Platforms = nil
	wantLint(t, baseCatalog(it), "a: platforms")
}

func TestLintScriptURL(t *testing.T) {
	a, b := validItem("a"), validItem("b")
	a.Install = []Method{{Backend: "script"}}
	b.Install = []Method{{Backend: "script", URL: "http://insecure.example/install.sh"}}
	c := baseCatalog(a, b)
	wantLint(t, c, "a: install[0]: url")
	wantLint(t, c, "b: install[0]: url")
}

func TestLintProfileExclusiveNotExclusive(t *testing.T) {
	c := baseCatalog(validItem("a"))
	c.Profiles = []Profile{{ID: "p", Exclusive: map[string]string{"shell": "a"}}}
	wantLint(t, c, "p: exclusive")
}

func TestLintValidCatalog(t *testing.T) {
	c := loadTestdata(t)
	if errs := Lint(c, testBackends); len(errs) != 0 {
		t.Fatalf("unexpected lint errors: %v", errs)
	}
}

func TestLintScriptInline(t *testing.T) {
	a := validItem("a")
	a.Install = []Method{{Backend: "script", Inline: "herdr plugin action invoke x"}}
	for _, e := range Lint(baseCatalog(a), testBackends) {
		t.Fatalf("inline script must be valid: %v", e)
	}
}

func TestLintVersionsDefaultFirst(t *testing.T) {
	it := validItem("a")
	it.Install[0].Version = "24"
	it.Install[0].Versions = []string{"22", "24"}
	wantLint(t, baseCatalog(it), "a: versions")
}
