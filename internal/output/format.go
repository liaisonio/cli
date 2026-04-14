package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

type Format string

const (
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
	FormatTable Format = "table"
)

// Parse normalises a user-supplied --output value. An empty string means
// "default", which for agent use is JSON.
func Parse(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "json":
		return FormatJSON, nil
	case "yaml", "yml":
		return FormatYAML, nil
	case "table":
		return FormatTable, nil
	}
	return "", fmt.Errorf("unknown output format %q (want json|yaml|table)", s)
}

// Print emits v in the requested format. If v is raw bytes (already JSON), it
// is either pretty-printed (JSON) or decoded first (YAML/table).
func Print(w io.Writer, f Format, v any) error {
	if w == nil {
		w = os.Stdout
	}
	switch f {
	case FormatJSON:
		return printJSON(w, v)
	case FormatYAML:
		return printYAML(w, v)
	case FormatTable:
		return fmt.Errorf("table output must use PrintTable")
	}
	return fmt.Errorf("unsupported format: %s", f)
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	switch x := v.(type) {
	case []byte:
		// Re-indent raw bytes so output is consistent.
		var any any
		if err := json.Unmarshal(x, &any); err != nil {
			_, err := w.Write(x)
			return err
		}
		return enc.Encode(any)
	default:
		return enc.Encode(v)
	}
}

func printYAML(w io.Writer, v any) error {
	switch x := v.(type) {
	case []byte:
		var any any
		if err := json.Unmarshal(x, &any); err != nil {
			return fmt.Errorf("decode json for yaml: %w", err)
		}
		return yaml.NewEncoder(w).Encode(any)
	default:
		return yaml.NewEncoder(w).Encode(v)
	}
}

// PrintTable writes a simple aligned table. headers is the column header row;
// rows are the data rows. Each row must have the same length as headers.
func PrintTable(w io.Writer, headers []string, rows [][]string) error {
	if w == nil {
		w = os.Stdout
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}
