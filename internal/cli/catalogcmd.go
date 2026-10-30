package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

func catalogCmd(e *Env) *cobra.Command {
	root := &cobra.Command{Use: "catalog", Short: i18n.T("cli.catalog")}
	var dir string
	lint := &cobra.Command{
		Use:   "lint",
		Short: i18n.T("cli.catalog_lint"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := e.Cat
			if dir != "" {
				var err error
				if c, err = catalog.Load(os.DirFS(dir)); err != nil {
					return exit(1, err.Error())
				}
			}
			errs := catalog.Lint(c, backend.Names())
			for _, err := range errs {
				fmt.Fprintln(e.Stdout, err)
			}
			if len(errs) > 0 {
				return exit(1, i18n.T("msg.lint_failed", len(errs)))
			}
			fmt.Fprintln(e.Stdout, i18n.T("msg.lint_ok", len(c.Items)))
			return nil
		},
	}
	lint.Flags().StringVar(&dir, "dir", "", i18n.T("flag.catalog_dir"))
	root.AddCommand(lint)
	return root
}
