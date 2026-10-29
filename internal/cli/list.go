package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/eusener/kanivet-cli/internal/i18n"
)

func listCmd(e *Env) *cobra.Command {
	var category string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("cli.list"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			type row struct {
				ID          string `json:"id"`
				Category    string `json:"category"`
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			var rows []row
			for _, id := range sortedItemIDs(e) {
				it := e.Cat.Items[id]
				if category != "" && it.Category != category {
					continue
				}
				rows = append(rows, row{it.ID, it.Category, it.Name.Get(), it.Description.Get()})
			}
			if asJSON {
				return writeJSON(e, rows)
			}
			w := tabwriter.NewWriter(e.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "ITEM\tCATEGORY\tDESCRIPTION")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.ID, r.Category, r.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&category, "category", "", i18n.T("flag.category"))
	cmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	return cmd
}

func profilesCmd(e *Env) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: i18n.T("cli.profiles"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			type row struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				Description string   `json:"description"`
				Items       []string `json:"items"`
			}
			var rows []row
			for _, p := range e.Cat.Profiles {
				rows = append(rows, row{p.ID, p.Name.Get(), p.Description.Get(), p.Items})
			}
			if asJSON {
				return writeJSON(e, rows)
			}
			w := tabwriter.NewWriter(e.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "PROFILE\tNAME\tDESCRIPTION")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.ID, r.Name, r.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	return cmd
}

func sortedItemIDs(e *Env) []string {
	order := map[string]int{}
	for _, c := range e.Cat.Categories {
		order[c.ID] = c.Order
	}
	ids := make([]string, 0, len(e.Cat.Items))
	for id := range e.Cat.Items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := e.Cat.Items[ids[i]], e.Cat.Items[ids[j]]
		if order[a.Category] != order[b.Category] {
			return order[a.Category] < order[b.Category]
		}
		return a.ID < b.ID
	})
	return ids
}

func writeJSON(e *Env, v any) error {
	enc := json.NewEncoder(e.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
