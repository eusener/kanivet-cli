// Package buildinfo informa a versão do binário: a da release (injetada por -ldflags),
// a do módulo num `go install …@vX.Y.Z` ou `dev` num build local.
package buildinfo

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

// Preenchidas pelo GoReleaser com -X; vazias num `go build` comum.
var (
	Version string
	Commit  string
	Date    string
)

const unknown = "unknown"

// pseudo reconhece pseudo-versões do Go (v0.0.0-20261030180405-9f030c0a1b2c), que o
// `go build` grava num clone fora de uma tag.
var pseudo = regexp.MustCompile(`-(0\.)?\d{14}-[0-9a-f]{12}$`)

// Info é a versão resolvida do binário.
type Info struct {
	Version, Commit, Date string
}

// String devolve "<versão> (<commit>, <data>)".
func (i Info) String() string {
	return fmt.Sprintf("%s (%s, %s)", i.Version, i.Commit, i.Date)
}

// Get resolve a versão deste binário.
func Get() Info { return resolve(Version, Commit, Date, debug.ReadBuildInfo) }

// String é atalho para Get().String().
func String() string { return Get().String() }

func resolve(v, commit, date string, read func() (*debug.BuildInfo, bool)) Info {
	info := Info{Version: v, Commit: commit, Date: date}
	if info.Version == "" {
		info.Version = "dev"
		if bi, ok := read(); ok {
			if mv := bi.Main.Version; mv != "" && mv != "(devel)" && !strings.Contains(mv, "+dirty") && !pseudo.MatchString(mv) {
				info.Version = strings.TrimPrefix(mv, "v")
			}
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					if info.Commit == "" {
						info.Commit = short(s.Value)
					}
				case "vcs.time":
					if info.Date == "" {
						info.Date = s.Value
					}
				}
			}
		}
	}
	if info.Commit == "" {
		info.Commit = unknown
	}
	info.Date = day(info.Date)
	return info
}

func short(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

// day reduz um timestamp RFC 3339 para AAAA-MM-DD.
func day(d string) string {
	if d == "" {
		return unknown
	}
	if len(d) >= 10 && d[4] == '-' && d[7] == '-' {
		return d[:10]
	}
	return d
}
