package config

import (
	"testing"
)

func TestJoin(t *testing.T) {
	tests := []struct {
		name string
		a    []any
		sep  string
		want string
	}{
		{name: "EmptySlice", a: []any{}, sep: ",", want: ""},
		{name: "OneElement", a: []any{1}, sep: ",", want: "1"},
		{name: "TwoElements", a: []any{1, 2}, sep: ",", want: "1,2"},
		{name: "ThreeElements", a: []any{1, 2, 3}, sep: ",", want: "1,2,3"},
		{name: "FourFloats", a: []any{1.1, 2.2, 3.3, 4.4}, sep: ",", want: "1.1,2.2,3.3,4.4"},
		{name: "FourStrings", a: []any{"a", "b", "c", "d"}, sep: ",", want: "a,b,c,d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := join(tt.a, tt.sep); got != tt.want {
				t.Errorf("join() = %v, want %v", got, tt.want)
			}
		})
	}
}
