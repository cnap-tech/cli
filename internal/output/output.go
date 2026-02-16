package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// Format controls output mode.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatQuiet Format = "quiet"
)

// PrintJSON writes v as indented JSON to stdout.
func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table creates a tab writer for aligned columns.
func Table(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

// PrintTable prints rows with a header using tabwriter.
func PrintTable(header []string, rows [][]string) {
	tw := Table(os.Stdout)
	for i, h := range header {
		if i > 0 {
			_, _ = fmt.Fprint(tw, "\t")
		}
		_, _ = fmt.Fprint(tw, h)
	}
	_, _ = fmt.Fprintln(tw)

	for _, row := range rows {
		for i, col := range row {
			if i > 0 {
				_, _ = fmt.Fprint(tw, "\t")
			}
			_, _ = fmt.Fprint(tw, col)
		}
		_, _ = fmt.Fprintln(tw)
	}
	_ = tw.Flush()
}
