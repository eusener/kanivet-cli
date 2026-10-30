package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

func doctorCmd(e *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: i18n.T("cli.doctor"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ok := func(b bool) string {
				if b {
					return e.icon(state.StepDone)
				}
				return e.icon(state.StepFailed)
			}
			p := e.P
			fmt.Fprintf(e.Stdout, "%s: %s %s %s %s (%s)\n", i18n.T("doctor.system"), p.OS, p.Arch, p.Distro, p.DistroVersion, p.PkgManager)
			fmt.Fprintf(e.Stdout, "%s: %s\n", i18n.T("doctor.shell"), p.Shell)
			rc, _ := os.ReadFile(e.rcPath())
			fmt.Fprintf(e.Stdout, "%s %s: %s\n", ok(strings.Contains(string(rc), configs.RcMarkerStart)), i18n.T("doctor.rc_block"), e.rcPath())
			localBin := filepath.Join(p.Home, ".local", "bin")
			inPath := false
			for _, d := range filepath.SplitList(e.Getenv["PATH"]) {
				inPath = inPath || d == localBin
			}
			fmt.Fprintf(e.Stdout, "%s %s\n", ok(inPath), i18n.T("doctor.local_bin", localBin))
			for _, bin := range []string{"brew", "mise", "git", "curl"} {
				res := e.R.Run(context.Background(), runner.Cmd{Shell: "command -v " + bin}, io.Discard)
				fmt.Fprintf(e.Stdout, "%s %s\n", ok(res.OK()), bin)
			}
			if j, _, _ := state.LatestUnfinished(e.StateDir); j != nil {
				fmt.Fprintln(e.Stdout, i18n.T("doctor.unfinished", j.RunID))
			}
			return nil
		},
	}
}
