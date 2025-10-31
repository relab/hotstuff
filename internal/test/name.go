// Package test provides utilities for generating test names based on fields and values.
package test

import (
	"fmt"
	"strings"
)

// Name returns the test or benchmark name based on the provided fields and values.
// If the first argument is a string and there is an odd number of arguments,
// it is treated as a prefix. Otherwise, all arguments are treated as field-value pairs.
// Example usages:
//
//	Name("MyPrefix", "field1", value1, "field2", value2)
//	Name("field1", value1, "field2", value2)
func Name(fieldValuePairs ...any) string {
	sep, prefix := "", ""
	if len(fieldValuePairs)%2 != 0 {
		if p, ok := fieldValuePairs[0].(string); ok {
			sep, prefix = "/", p
			fieldValuePairs = fieldValuePairs[1:]
		} else {
			panic("first argument must be a string when there is an odd number of arguments")
		}
	}

	b := strings.Builder{}
	b.WriteString(prefix)
	for i := 0; i < len(fieldValuePairs); i += 2 {
		field := fieldValuePairs[i]
		v := fieldValuePairs[i+1]
		switch value := v.(type) {
		case []string:
			if value == nil {
				b.WriteString(fmt.Sprintf("%s%s=<nil>", sep, field))
			} else {
				b.WriteString(fmt.Sprintf("%s%s=%v", sep, field, value))
			}
		case string:
			if value == "" {
				b.WriteString(fmt.Sprintf("%s%s=%q", sep, field, value))
			} else {
				b.WriteString(fmt.Sprintf("%s%s=%s", sep, field, value))
			}
		case uint64:
			b.WriteString(fmt.Sprintf("%s%s=%d", sep, field, value))
		case int:
			b.WriteString(fmt.Sprintf("%s%s=%d", sep, field, value))
		case bool:
			b.WriteString(fmt.Sprintf("%s%s=%t", sep, field, value))
		default:
			b.WriteString(fmt.Sprintf("%s%s=%v", sep, field, v))
		}
		// ensure separator is set to / after the first field
		sep = "/"
	}
	return b.String()
}
