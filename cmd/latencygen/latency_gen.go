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

var latencies = map[string]map[string]time.Duration{
`)
	for _, city := range keys {
		s.WriteString(fmt.Sprintf("%q: {\n", city))
		latencies := allToAllMatrix[city]
		for _, city2 := range keys {
			latency, err := time.ParseDuration(latencies[city2] + "ms")
			if err != nil {
				return nil, err
			}
			s.WriteString(fmt.Sprintf("%q: %d,\n", city2, int64(latency)))
		}
		s.WriteString(fmt.Sprintln("},"))
	}
	s.WriteString(fmt.Sprintln("}"))
	latenciesGoCode, err := format.Source([]byte(s.String()))
	if err != nil {
		return nil, err
	}
	return latenciesGoCode, err
}
