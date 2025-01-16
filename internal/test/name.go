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
	for i, f := range fields {
		v := values[i]
		switch x := v.(type) {
		case []string:
			if x == nil {
				b.WriteString(fmt.Sprintf("/%s=<nil>", f))
			} else {
				b.WriteString(fmt.Sprintf("/%s=%v", f, x))
			}
		case string:
			if x != "" {
				b.WriteString(fmt.Sprintf("/%s=%s", f, v))
			}
		case uint64:
			if x != 0 {
				b.WriteString(fmt.Sprintf("/%s=%d", f, v))
			}
		case int:
			if x != 0 {
				b.WriteString(fmt.Sprintf("/%s=%d", f, v))
			}
		case bool:
			if x {
				b.WriteString(fmt.Sprintf("/%s", f))
			}
		default:
			b.WriteString(fmt.Sprintf("/%s=%v", f, v))
		}
	}
	return b.String()
}
