// Package test provides utilities for generating test names based on fields and values.
package test

import (
	"fmt"
	"strings"
)

// Name returns the test name based on the provided fields and values.
func Name(fields []string, values ...any) string {
	if len(fields) != len(values) {
		panic("fields and values must have the same length")
	}
	b := strings.Builder{}
	sep := ""
	for i, f := range fields {
		v := values[i]
		switch x := v.(type) {
		case []string:
			if x == nil {
				b.WriteString(fmt.Sprintf("%s%s=<nil>", sep, f))
			} else {
				b.WriteString(fmt.Sprintf("%s%s=%v", sep, f, x))
			}
		case string:
			if x != "" {
				b.WriteString(fmt.Sprintf("%s%s=%s", sep, f, v))
			}
		case uint64:
			if x != 0 {
				b.WriteString(fmt.Sprintf("%s%s=%d", sep, f, v))
			}
		case int:
			if x != 0 {
				b.WriteString(fmt.Sprintf("%s%s=%d", sep, f, v))
			}
		case bool:
			if x {
				b.WriteString(fmt.Sprintf("%s%s", sep, f))
			}
		default:
			b.WriteString(fmt.Sprintf("%s%s=%v", sep, f, v))
		}
		// set separator to / after the first field
		sep = "/"
	}
	return b.String()
}
