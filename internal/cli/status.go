package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

func statusCmd(e *Env) *cobra.Command {
	var asJSON, checkUpdates bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: i18n.T("cli.status"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			sess, err := e.open()
			if err != nil {
				return err
			}
			ctx := context.Background()
			d := &detect.Detector{R: e.R, P: e.P, Owner: sess.St}
			statuses := d.Detect(ctx, e.Cat, nil)
			if checkUpdates {
				d.CheckUpdates(ctx, e.Cat, statuses)
			}
			type row struct {
				ID       string `json:"id"`
				Category string `json:"category"`
				Status   string `json:"status"`
				Version  string `json:"version"`
				Latest   string `json:"latest"`
				Reason   string `json:"reason"`
			}
			var rows []row
			for _, id := range sortedItemIDs(e) {
				s := statuses[id]
				rows = append(rows, row{id, e.Cat.Items[id].Category, s.Status.String(), s.Version, s.Latest, s.Reason})
			}
			if asJSON {
				return writeJSON(e, rows)
			}
			w := tabwriter.NewWriter(e.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "ITEM\tCATEGORY\tSTATUS\tVERSION\tLATEST")
			for _, r := range rows {
				status := r.Status
				if r.Reason != "" {
					status += " (" + r.Reason + ")"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Category, status, r.Version, r.Latest)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	cmd.Flags().BoolVar(&checkUpdates, "check-updates", false, i18n.T("flag.check_updates"))
	return cmd
}
