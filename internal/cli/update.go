package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
)

func updateCmd(e *Env) *cobra.Command {
	var r runFlags
	cmd := &cobra.Command{
		Use:   "update [item...]",
		Short: i18n.T("cli.update"),
		RunE: func(cmd *cobra.Command, args []string) error {
			sel := plan.Selection{Update: args, UpdateAll: len(args) == 0}
			return e.apply(r, func(s *app.Session) (*plan.Plan, error) {
				pl, _, err := s.Plan(context.Background(), sel)
				return pl, err
			}, "", false)
		},
	}
	r.bind(cmd)
	return cmd
}

func uninstallCmd(e *Env) *cobra.Command {
	var r runFlags
	var withOrphans bool
	cmd := &cobra.Command{
		Use:   "uninstall <item>...",
		Short: i18n.T("cli.uninstall"),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return e.apply(r, func(s *app.Session) (*plan.Plan, error) {
				sel := plan.Selection{Uninstall: args}
				pl, _, err := s.Plan(context.Background(), sel)
				if err == nil && withOrphans && len(pl.Orphans) > 0 {
					sel.Uninstall = append(sel.Uninstall, pl.Orphans...)
					pl, _, err = s.Plan(context.Background(), sel)
				}
				return pl, err
			}, "", false)
		},
	}
	r.bind(cmd)
	cmd.Flags().BoolVar(&withOrphans, "with-orphans", false, i18n.T("flag.with_orphans"))
	return cmd
}
