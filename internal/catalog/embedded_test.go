package catalog_test

import (
	"testing"

	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
)

var baseItems = []string{
	"kitty", "herdr", "herdr-plugins", "nerd-font-jetbrains", "herdr-icon-font", "starship", "zsh",
	"zsh-autosuggestions", "zsh-syntax-highlighting", "direnv", "atuin", "zoxide", "fzf", "eza", "bat",
	"fd", "ripgrep", "dust", "duf", "btop", "procs", "sd", "yazi", "television", "lazygit", "delta", "gh",
	"jj", "jq", "yq", "xh", "just", "hyperfine", "watchexec", "tldr", "mise",
}

func embedded(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEmbeddedCatalogLints(t *testing.T) {
	c := embedded(t)
	for _, err := range catalog.Lint(c, backend.Names()) {
		t.Error(err)
	}
	for _, id := range baseItems {
		if c.Items[id] == nil {
			t.Errorf("missing item %q", id)
		}
	}
}
