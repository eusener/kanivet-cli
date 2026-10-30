package i18n

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDetectLang(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want Lang
	}{
		{map[string]string{"LANG": "pt_BR.UTF-8"}, PtBR},
		{map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "pt_BR.UTF-8"}, En},
		{map[string]string{}, En},
	}
	for _, c := range cases {
		if got := DetectLang(c.env); got != c.want {
			t.Errorf("DetectLang(%v) = %q, want %q", c.env, got, c.want)
		}
	}
}

func TestTextYAML(t *testing.T) {
	defer SetLang(Current())
	var s Text
	if err := yaml.Unmarshal([]byte(`hello`), &s); err != nil || s[En] != "hello" {
		t.Fatalf("scalar: %v %v", s, err)
	}
	var m Text
	if err := yaml.Unmarshal([]byte("en: hi\npt-BR: oi\n"), &m); err != nil {
		t.Fatal(err)
	}
	SetLang(PtBR)
	if m.Get() != "oi" {
		t.Fatalf("pt-BR: got %q", m.Get())
	}
	var e Text
	_ = yaml.Unmarshal([]byte("en: hi\n"), &e)
	if e.Get() != "hi" {
		t.Fatalf("fallback: got %q", e.Get())
	}
}

func TestTMissingKey(t *testing.T) {
	if got := T("nope"); got != "nope" {
		t.Fatalf("got %q", got)
	}
}

func TestTMessagesParity(t *testing.T) {
	for k := range messages[En] {
		if _, ok := messages[PtBR][k]; !ok {
			t.Errorf("pt-BR missing %q", k)
		}
	}
	for k := range messages[PtBR] {
		if _, ok := messages[En][k]; !ok {
			t.Errorf("en missing %q", k)
		}
	}
}

func TestNPlural(t *testing.T) {
	t.Cleanup(func() { SetLang(En) })
	cases := []struct {
		lang      Lang
		key       string
		one, many string
	}{
		{En, "tui.dash.hd_updates", "1 update", "2 updates"},
		{En, "tui.profile.items", "1 item", "2 items"},
		{En, "tui.summary.tag.configs", "1 config", "2 configs"},
		{PtBR, "tui.dash.hd_installed", "1 instalado", "2 instalados"},
		{PtBR, "tui.dash.hd_updates", "1 atualização", "2 atualizações"},
		{PtBR, "tui.dash.hd_external", "1 externo", "2 externos"},
		{PtBR, "tui.summary.tag.installed", "1 instalado", "2 instalados"},
		{PtBR, "tui.summary.tag.failed", "1 falha", "2 falhas"},
		{PtBR, "tui.summary.tag.skipped", "1 pulado", "2 pulados"},
		{PtBR, "tui.summary.tag.conflicts", "1 mantida", "2 mantidas"},
		{PtBR, "tui.profile.items", "1 item", "2 itens"},
		{PtBR, "tui.welcome.sudo", "sudo em 1 item", "sudo em 2 itens"},
	}
	for _, c := range cases {
		SetLang(c.lang)
		if got := N(c.key, 1); got != c.one {
			t.Errorf("%s %s N=1: %q, want %q", c.lang, c.key, got, c.one)
		}
		if got := N(c.key, 2); got != c.many {
			t.Errorf("%s %s N=2: %q, want %q", c.lang, c.key, got, c.many)
		}
	}
	SetLang(PtBR)
	if got := N("tui.tag.install", 1); got != "1 a instalar" { // sem forma .one: usa a única
		t.Errorf("sem .one: %q", got)
	}
}
