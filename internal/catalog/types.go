// Package catalog define e carrega o catálogo declarativo de itens, categorias e perfis.
package catalog

import (
	"io/fs"

	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
)

const SchemaVersion = 1

// DefaultInitOrder é a posição do init de um item quando o catálogo não define outra.
const DefaultInitOrder = 50

type Catalog struct {
	Schema     int
	Version    string
	Categories []Category
	Items      map[string]*Item
	Profiles   []Profile
	Files      fs.FS // raiz do catálogo, para ler os templates
}

type Category struct {
	ID          string    `yaml:"id"`
	Name        i18n.Text `yaml:"name"`
	Description i18n.Text `yaml:"description"`
	Exclusive   bool      `yaml:"exclusive"`
	Order       int       `yaml:"-"`
}

type Item struct {
	ID          string            `yaml:"id"`
	Category    string            `yaml:"category"`
	Name        i18n.Text         `yaml:"name"`
	Description i18n.Text         `yaml:"description"`
	Notes       i18n.Text         `yaml:"notes"`
	Homepage    string            `yaml:"homepage"`
	Platforms   []platform.OS     `yaml:"platforms"`
	Requires    Requires          `yaml:"requires"`
	DependsOn   []string          `yaml:"depends_on"`
	ProvidedBy  []string          `yaml:"provided_by"`
	Provides    []string          `yaml:"provides"`
	MinVersion  string            `yaml:"min_version"`
	NeedsSudo   bool              `yaml:"needs_sudo"`
	ZshOnly     bool              `yaml:"zsh_only"`
	Detect      Detect            `yaml:"detect"`
	Install     []Method          `yaml:"install"`
	PostInstall []string          `yaml:"post_install"`
	Configs     []ConfigSpec      `yaml:"configs"`
	Shell       ShellSpec         `yaml:"shell"`
	Env         map[string]string `yaml:"env"`
}

type Requires struct {
	Systemd   bool     `yaml:"systemd"`
	Graphical bool     `yaml:"graphical"`
	MinMacOS  string   `yaml:"min_macos"`
	Distros   []string `yaml:"distros"` // "<id>" ou "<id>>=<versão>"
	Arch      []string `yaml:"arch"`
}

type Detect struct {
	Command      string `yaml:"command"`
	VersionRegex string `yaml:"version_regex"`
}

type Method struct {
	Backend string   `yaml:"backend"`
	Package string   `yaml:"package"`
	Version string   `yaml:"version"` // linha padrão ("22", "stable")
	When    *When    `yaml:"when"`
	URL     string   `yaml:"url"`
	Inline  string   `yaml:"inline"` // script: comando local, sem download
	SHA256  string   `yaml:"sha256"`
	Args    []string `yaml:"args"`
	Repo    string   `yaml:"repo"`
	Ref     string   `yaml:"ref"`
	Dest    string   `yaml:"dest"`
}

type When struct {
	OS         platform.OS `yaml:"os"`
	PkgManager string      `yaml:"pkg_manager"`
}

type ConfigSpec struct {
	ID          string       `yaml:"id"`
	Description i18n.Text    `yaml:"description"`
	Default     bool         `yaml:"default"`
	Files       []ConfigFile `yaml:"files"`
}

type ConfigFile struct {
	Template string `yaml:"template"`
	Target   string `yaml:"target"`
}

type ShellSpec struct {
	Guard     string            `yaml:"guard"` // binário que protege o init (padrão: 1ª palavra do detect)
	Aliases   map[string]string `yaml:"aliases"`
	Init      map[string]string `yaml:"init"` // "zsh" | "bash" | "any" (aceita {{shell}})
	InitOrder int               `yaml:"init_order"`
	Env       map[string]string `yaml:"env"`
}

type Profile struct {
	ID          string              `yaml:"id"`
	Name        i18n.Text           `yaml:"name"`
	Description i18n.Text           `yaml:"description"`
	Items       []string            `yaml:"items"`
	Exclusive   map[string]string   `yaml:"exclusive"`
	Extras      map[string][]string `yaml:"extras"`
}
