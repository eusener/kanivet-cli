package tui

import (
	"strings"
	"testing"

	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

func TestViewPaintedEveryScreen(t *testing.T) {
	exec, _ := executing(t)
	sum, _ := summaryModel(t)
	welcome := detected(t, newModel(t, linuxSystemd))
	profile := send(welcome, enter())
	cats := send(profile, enter())
	screens := map[string]Model{
		"welcome": welcome, "profile": profile, "categories": cats,
		"review": reviewFor(t, linuxSystemd), "execute": exec, "summary": sum,
		"dashboard": dashModel(t, map[string]detect.ItemStatus{"bat": st("bat", detect.StatusInstalled)}),
	}
	for name, m := range screens {
		m = send(m, size(100, 30))
		m.th = paperTheme()
		t.Run(name, func(t *testing.T) { assertPainted(t, m.View(), 100, 30) })
	}
}

func TestHeaderShowsTrail(t *testing.T) {
	m := send(detected(t, newModel(t, linuxSystemd)), enter(), enter())
	i18n.SetLang(i18n.PtBR)
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	m.th = paperTheme()
	m.w, m.h = 100, 30
	v := m.View()
	for _, want := range []string{"KANIVET", "ITENS", "3/5"} {
		if !strings.Contains(v, want) {
			t.Errorf("view sem %q:\n%s", want, v)
		}
	}
}

func TestTooSmallPainted(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m.th = paperTheme()
	m.w, m.h = 60, 30
	assertPainted(t, m.View(), 60, m.h)
}

func TestPrimaryAction(t *testing.T) {
	m := newModel(t, linuxSystemd)
	m.detecting = false
	if k, _, ok := m.primaryAction(); !ok || k != "enter" {
		t.Fatalf("welcome: key=%q ok=%v", k, ok)
	}
	m.detecting = true
	if _, _, ok := m.primaryAction(); ok {
		t.Fatal("welcome detectando não tem ação principal")
	}
	m.screen = ScreenCategories
	if _, _, ok := m.primaryAction(); ok {
		t.Fatal("categorias sem ação principal")
	}
}

// No modo plain sem Nerd Font a trilha é a mais larga; o cabeçalho de 80 colunas
// não pode cortar nem o último passo nem o FIM.
func TestHeaderFitsWidest80(t *testing.T) {
	i18n.SetLang(i18n.PtBR)
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	sum, _ := summaryModel(t)
	i18n.SetLang(i18n.PtBR)
	sum = send(sum, size(80, 24))
	v := sum.View()
	for _, want := range []string{"INSTALAR", "FIM"} {
		if !strings.Contains(v, want) {
			t.Errorf("cabeçalho sem %q:\n%s", want, v)
		}
	}
}
