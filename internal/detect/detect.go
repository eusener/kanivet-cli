// Package detect descobre o estado real de cada item do catálogo na máquina.
package detect

import (
	"context"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/version"
)

type Status int

const (
	StatusAbsent Status = iota
	StatusInstalled
	StatusOutdated
	StatusExternal
	StatusExternalOutdated
	StatusProvided
	StatusUnsupported
	StatusUnknown
)

var statusNames = [...]string{"absent", "installed", "outdated", "external", "external-outdated", "provided", "unsupported", "unknown"}

func (s Status) String() string { return statusNames[s] }

// Present diz se o item já existe na máquina (instalado por nós ou por fora).
func (s Status) Present() bool {
	switch s {
	case StatusInstalled, StatusOutdated, StatusExternal, StatusExternalOutdated, StatusUnknown:
		return true
	}
	return false
}

type ItemStatus struct {
	ID      string
	Status  Status
	Version string
	Latest  string
	Reason  string
}

type Owner interface{ Owned(id string) bool }

const DefaultTimeout = 5 * time.Second

type Detector struct {
	R       runner.Runner
	P       platform.Platform
	Owner   Owner
	Timeout time.Duration
}

// Detect devolve o estado de todos os itens; selected marca os itens escolhidos
// (usado para saber se um item é fornecido por outro, como php pelo lerd).
func (d *Detector) Detect(ctx context.Context, c *catalog.Catalog, selected map[string]bool) map[string]ItemStatus {
	out := map[string]ItemStatus{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, it := range c.Items {
		wg.Add(1)
		go func(it *catalog.Item) {
			defer wg.Done()
			sem <- struct{}{}
			s := d.detectOne(ctx, it)
			<-sem
			mu.Lock()
			out[it.ID] = s
			mu.Unlock()
		}(it)
	}
	wg.Wait()

	for _, it := range c.Items {
		s := out[it.ID]
		if s.Status.Present() || s.Status == StatusUnsupported {
			continue
		}
		for _, p := range it.ProvidedBy {
			if selected[p] || out[p].Status.Present() {
				out[it.ID] = ItemStatus{ID: it.ID, Status: StatusProvided, Reason: p}
				break
			}
		}
	}
	return out
}

func (d *Detector) detectOne(ctx context.Context, it *catalog.Item) ItemStatus {
	s := ItemStatus{ID: it.ID}
	if reason := Unsupported(it, d.P); reason != "" {
		s.Status, s.Reason = StatusUnsupported, reason
		return s
	}
	timeout := d.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res := d.R.Run(cctx, runner.Cmd{Shell: it.Detect.Command}, io.Discard)
	if cctx.Err() != nil {
		s.Status = StatusUnknown
		return s
	}
	if !res.OK() {
		s.Status = StatusAbsent
		return s
	}
	if it.Detect.VersionRegex != "" {
		s.Version, _ = version.Extract(res.Output, it.Detect.VersionRegex)
	}
	owned := d.Owner != nil && d.Owner.Owned(it.ID)
	tooOld := s.Version != "" && it.MinVersion != "" && version.Compare(s.Version, it.MinVersion) < 0
	switch {
	case owned:
		s.Status = StatusInstalled
	case tooOld:
		s.Status = StatusExternalOutdated
	default:
		s.Status = StatusExternal
	}
	return s
}

// Unsupported devolve o motivo pelo qual o item não roda nesta plataforma ("" = suportado).
func Unsupported(it *catalog.Item, p platform.Platform) string {
	if !containsOS(it.Platforms, p.OS) {
		return i18n.T("reason.platform", string(p.OS))
	}
	r := it.Requires
	if len(r.Arch) > 0 && !contains(r.Arch, p.Arch) {
		return i18n.T("reason.arch", p.Arch)
	}
	if r.Graphical && !p.Graphical {
		return i18n.T("reason.graphical")
	}
	if p.OS == platform.MacOS && r.MinMacOS != "" && version.Compare(p.DistroVersion, r.MinMacOS) < 0 {
		return i18n.T("reason.macos", r.MinMacOS)
	}
	if p.OS == platform.Linux {
		if r.Systemd && !p.Systemd {
			return i18n.T("reason.systemd")
		}
		if len(r.Distros) > 0 && !distroMatches(r.Distros, p) {
			return i18n.T("reason.distro", strings.Join(r.Distros, ", "))
		}
	}
	return ""
}

func distroMatches(specs []string, p platform.Platform) bool {
	for _, spec := range specs {
		id, min, _ := strings.Cut(spec, ">=")
		if id == p.Distro && (min == "" || version.Compare(p.DistroVersion, min) >= 0) {
			return true
		}
	}
	return false
}

// CheckUpdates consulta os backends e marca como Outdated os itens nossos com versão nova.
func (d *Detector) CheckUpdates(ctx context.Context, c *catalog.Catalog, st map[string]ItemStatus) {
	byBackend := map[string][]catalog.Method{}
	methodOf := map[string]catalog.Method{}
	for id, s := range st {
		if s.Status != StatusInstalled {
			continue
		}
		m, b, err := backend.Choose(c.Items[id], d.P)
		if err != nil {
			continue
		}
		byBackend[b.Name()] = append(byBackend[b.Name()], m)
		methodOf[id] = m
	}
	names := make([]string, 0, len(byBackend))
	for n := range byBackend {
		names = append(names, n)
	}
	sort.Strings(names)
	latest := map[string]map[string]string{}
	for _, n := range names {
		b, _ := backend.Get(n)
		latest[n] = b.Latest(ctx, d.R, byBackend[n])
	}
	for id, m := range methodOf {
		v := latest[m.Backend][m.Package]
		if v == "" {
			continue
		}
		s := st[id]
		s.Latest = v
		if s.Version == "" || version.Compare(s.Version, v) < 0 {
			s.Status = StatusOutdated
		}
		st[id] = s
	}
}

func containsOS(list []platform.OS, os platform.OS) bool {
	for _, x := range list {
		if x == os {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
