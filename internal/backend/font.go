package backend

import (
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

// font baixa um .zip de fontes e extrai os .ttf na pasta de fontes do usuário.
type font struct{ base }

func init() { register(font{}) }

func (font) Name() string { return "font" }

func FontDir(p platform.Platform) string {
	if p.OS == platform.MacOS {
		return p.Home + "/Library/Fonts"
	}
	return p.Home + "/.local/share/fonts"
}

func (font) Install(m catalog.Method, _ string, p platform.Platform) []runner.Cmd {
	dir := q(FontDir(p))
	sh := `T=$(mktemp -d) && curl -fsSL ` + q(m.URL) + ` -o "$T/f.zip" && mkdir -p ` + dir +
		` && unzip -o -j "$T/f.zip" '*.ttf' -d ` + dir
	if p.OS == platform.Linux {
		sh += ` && fc-cache -f ` + dir
	}
	sh += `; rc=$?; rm -rf "$T"; exit $rc`
	return []runner.Cmd{{Shell: sh, Display: "font " + m.URL}}
}

func (f font) Update(m catalog.Method, p platform.Platform) []runner.Cmd { return f.Install(m, "", p) }
