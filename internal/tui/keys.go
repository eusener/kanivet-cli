package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

// KeyMap reúne os atalhos da TUI. ShortHelp/FullHelp vêm das telas.
type KeyMap struct {
	Up, Down, Left, Right, Toggle, Enter, Back, Search, Help, Quit, Apply, Update, UpdateAll, Diff, Profile, Follow, Interrupt, Force, Log, Retry key.Binding
}

func DefaultKeys() KeyMap {
	b := func(keys []string, label, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, i18n.T(desc)))
	}
	return KeyMap{
		Up:        b([]string{"up", "k"}, "↑/k", "tui.key.up"),
		Down:      b([]string{"down", "j"}, "↓/j", "tui.key.down"),
		Left:      b([]string{"left", "h"}, "←/h", "tui.key.left"),
		Right:     b([]string{"right", "l"}, "→/l", "tui.key.right"),
		Toggle:    b([]string{" "}, "space", "tui.key.toggle"),
		Enter:     b([]string{"enter"}, "enter", "tui.key.enter"),
		Back:      b([]string{"esc"}, "esc", "tui.key.back"),
		Search:    b([]string{"/"}, "/", "tui.key.search"),
		Help:      b([]string{"?"}, "?", "tui.key.help"),
		Quit:      b([]string{"q", "ctrl+c"}, "q", "tui.key.quit"),
		Apply:     b([]string{"a"}, "a", "tui.key.apply"),
		Update:    b([]string{"u"}, "u", "tui.key.update"),
		UpdateAll: b([]string{"U"}, "U", "tui.key.update_all"),
		Diff:      b([]string{"d"}, "d", "tui.key.diff"),
		Profile:   b([]string{"p"}, "p", "tui.key.profile"),
		Follow:    b([]string{"f"}, "f", "tui.key.follow"),
		Force:     b([]string{"F"}, "F", "tui.key.force"),
		Log:       b([]string{"l"}, "l", "tui.key.log"),
		Retry:     b([]string{"r"}, "r", "tui.key.retry"),
		Interrupt: b([]string{"ctrl+c"}, "ctrl+c", "tui.key.interrupt"),
	}
}
