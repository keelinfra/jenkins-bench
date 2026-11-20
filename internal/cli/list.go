package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/keelinfra/jenkins-bench/internal/checks"
	"github.com/keelinfra/jenkins-bench/internal/console"
)

// loadChecks is checks.Load; a variable so a test can stand in a bundle that
// fails to load.
var loadChecks = checks.Load

func newListChecksCommand() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list-checks",
		Short: "List the controls in the policy bundle",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bundle, err := loadChecks()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				return enc.Encode(bundle.Checks)
			}

			if err := writeCheckList(out, bundle.Checks, isTerminal(out) && !hasNoColorEnv()); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n%d controls\n", len(bundle.Checks))
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print the full check metadata as JSON")
	return cmd
}

// writeCheckList prints the bundle as aligned columns: the same words with
// colour off, the IDs and severities painted the way the scan report paints
// them with it on, so a control looks the same in both places.
//
// Padding is computed on the plain text and the colour wrapped around it
// afterwards; tabwriter would count the escapes as width and push every
// painted column out of line.
func writeCheckList(out io.Writer, list []checks.Check, colour bool) error {
	p := console.Painter{Enabled: colour}
	header := []string{"ID", "SEVERITY", "SCOPE", "AUTOMATED", "TITLE"}
	rows := [][]string{}
	for _, c := range list {
		automated := "yes"
		if !c.Automated {
			automated = "no (manual)"
		}
		rows = append(rows, []string{c.ID, strings.ToUpper(c.Severity), c.Scope, automated, c.Title})
	}
	widths := make([]int, len(header))
	for _, r := range append([][]string{header}, rows...) {
		for i, cell := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}
	paint := func(i int, cell string) string {
		switch i {
		case 0:
			return p.Paint(console.Cyan, cell)
		case 1:
			switch cell {
			case checks.SeverityHigh:
				return p.Paint(console.Red, cell)
			case checks.SeverityMedium:
				return p.Paint(console.Yellow, cell)
			}
			return p.Paint(console.Dim, cell)
		case 3:
			if cell != "yes" {
				return p.Paint(console.Dim, cell)
			}
		}
		return cell
	}
	var b strings.Builder
	writeRow := func(r []string, paint func(int, string) string) {
		for i, cell := range r {
			b.WriteString(paint(i, cell))
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+2))
			}
		}
		b.WriteString("\n")
	}
	writeRow(header, func(_ int, cell string) string { return p.Paint(console.Bold, cell) })
	for _, r := range rows {
		writeRow(r, paint)
	}
	_, err := io.WriteString(out, b.String())
	return err
}
