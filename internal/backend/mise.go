package backend

import (
	"context"
	"encoding/json"
	"io"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

type mise struct{ base }

func init() { register(mise{}) }

func (mise) Name() string { return "mise" }

func (mise) Install(m catalog.Method, version string, _ platform.Platform) []runner.Cmd {
	if version == "" {
		version = m.Version
	}
	if version == "" {
		version = "latest"
	}
	return []runner.Cmd{argv("mise", "mise", "use", "-g", m.Package+"@"+version)}
}

func (mise) Update(m catalog.Method, _ platform.Platform) []runner.Cmd {
	return []runner.Cmd{argv("mise", "mise", "upgrade", m.Package)}
}

func (mise) Uninstall(m catalog.Method, _ platform.Platform) []runner.Cmd {
	return []runner.Cmd{
		argv("mise", "mise", "uninstall", "--all", m.Package),
		argv("mise", "mise", "unuse", "-g", m.Package),
	}
}

func (mise) Latest(ctx context.Context, r runner.Runner, ms []catalog.Method) map[string]string {
	res := r.Run(ctx, argv("mise", "mise", "outdated", "--json"), io.Discard)
	if !res.OK() {
		return nil
	}
	var out map[string]struct {
		Latest string `json:"latest"`
	}
	if json.Unmarshal([]byte(res.Output), &out) != nil {
		return nil
	}
	got := map[string]string{}
	for _, m := range ms {
		if o, ok := out[m.Package]; ok {
			got[m.Package] = o.Latest
		}
	}
	return got
}
