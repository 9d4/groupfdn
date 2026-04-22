package output

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/olekukonko/tablewriter"
)

// Format represents output format type
type Format string

const (
	FormatTable  Format = "table"
	FormatSimple Format = "simple"
	FormatJSON   Format = "json"
)

// Formatter handles output formatting
type Formatter struct {
	format Format
}

// NewFormatter creates a new formatter
func NewFormatter(format Format) *Formatter {
	return &Formatter{format: format}
}

// Print outputs data in the specified format
func (f *Formatter) Print(data interface{}, headers []string) {
	switch f.format {
	case FormatJSON:
		f.printJSON(data)
	case FormatSimple:
		f.printSimple(data, headers)
	case FormatTable:
		f.printTable(data, headers)
	default:
		f.printTable(data, headers)
	}
}

// PrintMap outputs a single map/object
func (f *Formatter) PrintMap(data map[string]interface{}) {
	switch f.format {
	case FormatJSON:
		f.printJSON(data)
	case FormatSimple:
		f.printMapSimple(data)
	case FormatTable:
		f.printMapTable(data)
	default:
		f.printMapTable(data)
	}
}

// PrintMessage outputs a simple message
func (f *Formatter) PrintMessage(msg string) {
	if f.format == FormatJSON {
		f.printJSON(map[string]string{"message": msg})
	} else {
		fmt.Println(msg)
	}
}

// PrintError outputs an error message
func (f *Formatter) PrintError(err error) {
	if f.format == FormatJSON {
		f.printJSON(map[string]string{"error": err.Error()})
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

func (f *Formatter) printJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(data)
}

func (f *Formatter) printTable(data interface{}, headers []string) {
	table := tablewriter.NewWriter(os.Stdout)

	// Configure table
	if len(headers) > 0 {
		table.Header(headers)
	}

	rows := f.extractRows(data, headers)
	if len(rows) > 0 {
		table.Bulk(rows)
	}

	table.Render()
}

func (f *Formatter) printSimple(data interface{}, headers []string) {
	rows := f.extractRows(data, headers)
	for _, row := range rows {
		fmt.Println(strings.Join(row, "\t"))
	}
}

func (f *Formatter) printMapTable(data map[string]interface{}) {
	table := tablewriter.NewWriter(os.Stdout)

	for key, value := range data {
		table.Append([]string{key, fmt.Sprintf("%v", value)})
	}
	table.Render()
}

func (f *Formatter) printMapSimple(data map[string]interface{}) {
	for key, value := range data {
		fmt.Printf("%s:\t%v\n", key, value)
	}
}

func (f *Formatter) extractRows(data interface{}, headers []string) [][]string {
	var rows [][]string

	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Slice {
		return rows
	}

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		row := f.extractRow(elem.Interface(), headers)
		rows = append(rows, row)
	}

	return rows
}

func (f *Formatter) extractRow(data interface{}, headers []string) []string {
	row := make([]string, len(headers))

	switch v := data.(type) {
	case map[string]interface{}:
		for i, h := range headers {
			if val, ok := v[h]; ok {
				row[i] = fmt.Sprintf("%v", val)
			} else {
				row[i] = ""
			}
		}
	default:
		// Try reflection for structs
		rv := reflect.ValueOf(data)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		if rv.Kind() == reflect.Struct {
			for i, h := range headers {
				field := rv.FieldByNameFunc(func(name string) bool {
					return strings.EqualFold(name, h) || strings.EqualFold(name, f.toCamelCase(h))
				})
				if field.IsValid() {
					row[i] = fmt.Sprintf("%v", field.Interface())
				} else {
					row[i] = ""
				}
			}
		}
	}

	return row
}

func (f *Formatter) toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}
