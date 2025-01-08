package config

import (
	"fmt"
	"strconv"
	"strings"
)

// join concatenates the elements of a to create a single string with elements separated by sep.
func join[T any](a []T, sep string) string {
	return strings.Trim(strings.ReplaceAll(fmt.Sprint(a), " ", sep), "[]")
}

func (c *Config) String() string {
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
		s.WriteString(strconv.Itoa(c.BranchFactor))
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
