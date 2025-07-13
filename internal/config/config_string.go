package config

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// join concatenates the elements of a to create a single string with elements separated by sep.
func join[T any](a []T, sep string) string {
	return strings.Trim(strings.ReplaceAll(fmt.Sprint(a), " ", sep), "[]")
}

func (c *ExperimentConfig) String() string {
	s := strings.Builder{}
	s.WriteString("ReplicaHosts: ")
	s.WriteString(strings.Join(c.ReplicaHosts, ", "))
	s.WriteString(", ")
	s.WriteString("ClientHosts: ")
	s.WriteString(strings.Join(c.ClientHosts, ", "))
	s.WriteString(", ")
	s.WriteString("Replicas: ")
	s.WriteString(strconv.Itoa(c.Replicas))
	s.WriteString(", ")
	s.WriteString("Clients: ")
	s.WriteString(strconv.Itoa(c.Clients))
	if len(c.Locations) == 0 {
		return s.String()
	}
	s.WriteString(", ")
	s.WriteString("Locations: ")
	s.WriteString(strings.Join(c.Locations, ", "))
	if len(c.TreePositions) > 0 {
		s.WriteString(", ")
		s.WriteString("TreePositions: ")
		s.WriteString(join(c.TreePositions, ", "))
		s.WriteString(", ")
		s.WriteString("BranchFactor: ")
		s.WriteString(strconv.Itoa(int(c.BranchFactor)))
	}
	if len(c.ByzantineStrategy) == 0 {
		return s.String()
	}
	s.WriteString(", ")
	s.WriteString("ByzantineStrategy: ")
	s.WriteString("{")
	for strategy, ids := range c.ByzantineStrategy {
		s.WriteString(strategy)
		s.WriteString(": ")
		s.WriteString(join(ids, ", "))
		s.WriteString(", ")
	}
	s.WriteString("}")
	return s.String()
}

// configurationTemplate holds separate slices for slice-fields and scalar-fields, plus key-column width.
type configurationTemplate struct {
	SliceKeys  []string
	SliceVals  []string
	ScalarKeys []string
	ScalarVals []string
	KeyWidths  [3]int
	ValWidths  [3]int
}

// addSliceField adds a slice/map field to the template data
func (td *configurationTemplate) addSliceField(name, value string) {
	td.SliceKeys = append(td.SliceKeys, name)
	td.SliceVals = append(td.SliceVals, value)
}

// addScalarField adds a scalar field to the template data
func (td *configurationTemplate) addScalarField(name, value string) {
	td.ScalarKeys = append(td.ScalarKeys, name)
	td.ScalarVals = append(td.ScalarVals, value)
}

// computeColumnWidths based on the longest key for the key column and
// longest value for the value column.
func (td *configurationTemplate) computeColumnWidths() {
	numRows := (len(td.ScalarKeys) + 2) / 3

	for i, key := range td.ScalarKeys {
		col := i / numRows // the column this index belongs to
		td.KeyWidths[col] = max(td.KeyWidths[col], len(key))
	}
	for i, val := range td.ScalarVals {
		col := i / numRows // the column this index belongs to
		td.ValWidths[col] = max(td.ValWidths[col], len(val))
	}
}

// formatSliceLines renders slice fields manually.
func (td *configurationTemplate) formatSliceLines() []string {
	var sliceLines []string
	// determine the width of the slice key column based on the longest key
	var sliceKeyWidth int
	for _, key := range td.SliceKeys {
		if len(key) > sliceKeyWidth {
			sliceKeyWidth = len(key)
		}
	}
	for i, key := range td.SliceKeys {
		line := fmt.Sprintf("%-*s: %s", sliceKeyWidth, key, td.SliceVals[i])
		sliceLines = append(sliceLines, line)
	}
	return sliceLines
}

const columnSeparatorWidth = 4

// formatScalarLines builds scalar fields in column-first order
func (td *configurationTemplate) formatScalarLines() []string {
	var lines []string
	numRows := (len(td.ScalarKeys) + 2) / 3

	for row := range numRows {
		var b strings.Builder
		for col := range 3 {
			i := row + col*numRows
			if i < len(td.ScalarKeys) {
				key, val := td.ScalarKeys[i], td.ScalarVals[i]
				// format: key (left-aligned) + ": " + value (left-aligned)
				b.WriteString(fmt.Sprintf("%-*s: %-*s", td.KeyWidths[col], key, td.ValWidths[col], val))
				// add separator if not the last column
				if col < 2 {
					b.WriteString(strings.Repeat(" ", columnSeparatorWidth))
				}
			}
		}
		lines = append(lines, b.String())
	}
	return lines
}

// PrettyPrint formats ExperimentConfig into a star-bordered box:
// - slice fields and map fields: one column, printed first
// - scalar fields: three columns, ordered top-to-bottom, printed after separator
// - maxWidth: cap with ellipsis
func (c ExperimentConfig) PrettyPrint(maxWidth int) string {
	v := reflect.ValueOf(c)
	t := v.Type()
	var sd configurationTemplate
	for i := range v.NumField() {
		name := t.Field(i).Name
		fv := v.Field(i).Interface()
		switch x := fv.(type) {
		case []string:
			val := formatSliceString(x, maxWidth-len(name)-2)
			sd.addSliceField(name, val)
		case []uint32:
			val := formatSliceUint32(x, maxWidth-len(name)-2)
			sd.addSliceField(name, val)
		case map[string][]uint32:
			val := formatMapStringSliceUint32(x, maxWidth-len(name)-2)
			sd.addSliceField(name, val)
		default:
			val := fmt.Sprint(fv)
			sd.addScalarField(name, val)
		}
	}

	sd.computeColumnWidths()
	sliceLines := sd.formatSliceLines()
	scalarLines := sd.formatScalarLines()

	width := max(longestLine(sliceLines), longestLine(scalarLines))

	allLines := make([]string, 0, len(sliceLines)+len(scalarLines)+1)
	allLines = append(allLines, sliceLines...)
	allLines = append(allLines, strings.Repeat("-", width))
	allLines = append(allLines, scalarLines...)

	// print border and experiment configuration lines
	border := strings.Repeat("*", width)
	var out strings.Builder
	out.WriteString(border + "\n")
	for _, line := range allLines {
		out.WriteString(line + "\n")
	}
	out.WriteString(border + "\n")
	return out.String()
}

// longestLine returns the length of the longest line in s.
func longestLine(s []string) int {
	longest := slices.MaxFunc(s, func(a, b string) int {
		return len(a) - len(b)
	})
	return len(longest)
}

// formatSliceString formats a []string into "[a b c … z]" with ellipsis if too long.
func formatSliceString(s []string, maxLen int) string {
	// check if full string fits within maxLen
	if f := fmt.Sprintf("%s", s); len(f) <= maxLen {
		return f
	}
	// since we're past the first check, we know we need ellipsis.

	// Find the maximum number of elements that can fit in the string,
	// including the first element, ellipsis, and the last element.
	var parts []string
	lastElem := len(s) - 1
	for i := range lastElem { // iterate over all but the last element
		// try adding the current element with ellipsis and last element
		testParts := append(parts, s[i], "…", s[lastElem])
		// if adding this element makes it too long, stop here
		if f := fmt.Sprintf("%s", testParts); len(f) > maxLen {
			break
		}
		// otherwise, add this element to our parts
		parts = append(parts, s[i])
	}
	// couldn't fit any elements, use the minimal representation: [first … last]
	if len(parts) == 0 {
		parts = append(parts, s[0])
	}
	// add ellipsis and last element
	parts = append(parts, "…", s[lastElem])
	return fmt.Sprintf("%s", parts)
}

// formatSliceUint32 formats a []uint32 into “[1 2 3 … 9]” with ellipsis if too long.
func formatSliceUint32(s []uint32, maxLen int) string {
	str := make([]string, len(s))
	for i, v := range s {
		str[i] = fmt.Sprint(v)
	}
	return formatSliceString(str, maxLen)
}

// formatMapStringSliceUint32 formats a map[string][]uint32 into a string representation.
func formatMapStringSliceUint32(m map[string][]uint32, maxLen int) string {
	if len(m) == 0 {
		return "{}"
	}
	var parts []string
	for key, vals := range m {
		valStr := formatSliceUint32(vals, maxLen/2) // Use half maxLen for each value
		parts = append(parts, fmt.Sprintf("%s: %s", key, valStr))
	}
	out := "{" + strings.Join(parts, ", ") + "}"
	if len(out) <= maxLen {
		return out
	}
	// If too long, just show the first key-value pair
	if len(parts) > 0 {
		return "{" + parts[0] + ", …}"
	}
	return "{…}"
}
