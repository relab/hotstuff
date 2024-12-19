// LatencyGen generates a Go source file containing the latency matrix.
package main

import (
	_ "embed"
	"encoding/csv"
	"flag"
	"fmt"
	"go/format"
	"log"
	"maps"
	"os"
	"slices"
	"strings"
	"time"
)

//go:embed latencies.csv
var csvLatencies string

//go:generate go run .

func main() {
	dstFile := flag.String("dest", "../../internal/latencies/latency_matrix.go", "file path to save latencies to.")
	flag.Parse()
	if *dstFile == "" {
		flag.Usage()
		os.Exit(1)
	}
	allToAllMatrix, err := parseCSVMatrix(csvLatencies)
	if err != nil {
		log.Fatal(err)
	}
	latenciesGoCode, err := latenciesToGoCode(allToAllMatrix)
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(*dstFile, latenciesGoCode, 0o600); err != nil {
		log.Fatal(err)
	}
}

func parseCSVMatrix(csvData string) (map[string]map[string]string, error) {
	reader := csv.NewReader(strings.NewReader(csvData))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV data: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("incomplete CSV data")
	}
	// Extract city names from the first row
	cities := rows[0][1:] // Skip the first column as it's the row headers
	if len(cities) == 0 {
		return nil, fmt.Errorf("no cities found in the header row")
	}

	allToAllMatrix := make(map[string]map[string]string, len(cities))
	for _, row := range rows[1:] { // Skip the header row
		if len(row) < 2 {
			continue // Skip empty or malformed rows
		}
		// First column is the city name for this row
		rowCity := row[0]
		if allToAllMatrix[rowCity] == nil {
			allToAllMatrix[rowCity] = make(map[string]string, len(cities))
		}
		// Populate the matrix for this row
		for i, latency := range row[1:] {
			colCity := cities[i]
			allToAllMatrix[rowCity][colCity] = latency
		}
	}
	return allToAllMatrix, nil
}

func latenciesToGoCode(allToAllMatrix map[string]map[string]string) ([]byte, error) {
	keys := slices.Sorted(maps.Keys(allToAllMatrix))

	s := strings.Builder{}
	s.WriteString(`package latencies

import "time"

var cities = []string{`)
	var longestCityName int
	// Write the city names to the `cities` slice
	for _, city := range keys {
		if len(city) > longestCityName {
			longestCityName = len(city)
		}
		s.WriteString(fmt.Sprintf("%q, ", city))
	}
	s.WriteString("}\n\n")

	// Write the 2D latency slice
	s.WriteString("// one-way latencies between cities.\n")
	s.WriteString("var latencies = [][]time.Duration{\n")
	for idx, city := range keys {
		paddedCity := fmt.Sprintf("%-*s", longestCityName, city)
		s.WriteString(fmt.Sprintf("\t/* %03d: %s */ {", idx, paddedCity))
		latencies := allToAllMatrix[city]
		for _, city2 := range keys {
			// Parse the latency as time.Duration
			latency, err := time.ParseDuration(latencies[city2] + "ms")
			if err != nil {
				return nil, fmt.Errorf("invalid latency between %s and %s: %w", city, city2, err)
			}
			// Divide the round-trip latency by 2 to simulate one-way latency
			s.WriteString(fmt.Sprintf("%d, ", latency/2))
		}
		s.WriteString("},\n")
	}
	s.WriteString("}\n")

	latenciesGoCode, err := format.Source([]byte(s.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to format Go code: %w", err)
	}
	return latenciesGoCode, nil
}
