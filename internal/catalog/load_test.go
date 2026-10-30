package catalog

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func loadTestdata(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load(os.DirFS("testdata"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadOrdersCategories(t *testing.T) {
	c := loadTestdata(t)
	if len(c.Categories) != 2 || c.Categories[0].ID != "shell" || c.Categories[1].ID != "php-local" {
		t.Fatalf("categories: %+v", c.Categories)
	}
	if !c.Categories[1].Exclusive || c.Categories[1].Order != 1 {
		t.Fatalf("php-local: %+v", c.Categories[1])
	}
}

func TestLoadItemFields(t *testing.T) {
	c := loadTestdata(t)
	fzf := c.Items["fzf"]
	if fzf == nil || fzf.Install[0].Backend != "brew" || fzf.Shell.InitOrder != 55 {
		t.Fatalf("fzf: %+v", fzf)
	}
	if fzf.Install[0].When == nil || fzf.Install[0].When.OS != "macos" || fzf.MinVersion != "0.48" {
		t.Fatalf("fzf fields: %+v", fzf)
	}
	if c.Items["lerd"].Shell.InitOrder != 50 {
		t.Fatalf("default init order: %d", c.Items["lerd"].Shell.InitOrder)
	}
	if c.Profiles[0].Exclusive["php-local"] != "lerd" {
		t.Fatalf("profile: %+v", c.Profiles[0])
	}
}

func TestLoadRejectsNewerSchema(t *testing.T) {
	fs := fstest.MapFS{"catalog.yaml": {Data: []byte("schema: 2\ncategories: []\n")}}
	if _, err := Load(fs); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("want schema error, got %v", err)
	}
}

func TestLoadDuplicateID(t *testing.T) {
	item := []byte("id: x\ncategory: c\nname: x\ndescription: x\nplatforms: [linux]\n")
	fs := fstest.MapFS{
		"catalog.yaml": {Data: []byte("schema: 1\ncategories: []\n")},
		"items/a.yaml": {Data: item},
		"items/b.yaml": {Data: item},
	}
	if _, err := Load(fs); err == nil || !strings.Contains(err.Error(), `"x"`) {
		t.Fatalf("want duplicate error naming x, got %v", err)
	}
}

func TestEmbeddedLoads(t *testing.T) {
	if _, err := Embedded(); err != nil {
		t.Fatal(err)
	}
}
