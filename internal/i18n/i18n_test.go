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
