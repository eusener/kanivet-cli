package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/state"
)

func restoreCmd(e *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "restore [run-id]",
		Short: i18n.T("cli.restore"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			backups := filepath.Join(e.StateDir, "backups")
			if len(args) == 0 {
				runs, err := state.Runs(e.StateDir)
				if err != nil {
					return err
				}
				found := false
				for _, id := range runs {
					if _, err := os.Stat(filepath.Join(backups, id)); err == nil {
						fmt.Fprintln(e.Stdout, id)
						found = true
					}
				}
				if !found {
					fmt.Fprintln(e.Stdout, i18n.T("msg.no_backups"))
				}
				return nil
			}
			dir := filepath.Join(backups, filepath.Base(args[0]))
			if _, err := os.Stat(dir); err != nil {
				return exit(1, i18n.T("err.unknown_run", args[0]))
			}
			restored, err := configs.RestoreRun(dir)
			if err != nil {
				return err
			}
			for _, f := range restored {
				fmt.Fprintln(e.Stdout, "  ← "+f)
			}
			return nil
		},
	}
}
