// LatencyGen generates a Go source file containing the latency matrix.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/maps"
)

const latencyMatrix = `{"Cape Town": {"Cape Town": "12.22", "Hong Kong": "255.05", "Tokyo": "358.16", "Seoul": "386.8", "Osaka": "364.07", "Mumbai": "161.49", "Singapore": "218.27", "Sydney": "410.86", "Central": "226.52", "Frankfurt": "157.04", "Stockholm": "180.28", "Milan": "164.98", "Ireland": "159.97", "London": "152.14", "Paris": "147.37", "Bahrain": "207.85", "Sao Paulo": "340.38", "N. Virginia": "228.02", "Ohio": "238.61", "N. California": "286.84", "Oregon": "274.93"}, "Hong Kong": {"Cape Town": "302.15", "Hong Kong": "3.17", "Tokyo": "53.59", "Seoul": "37.95", "Osaka": "50.32", "Mumbai": "91.24", "Singapore": "37.74", "Sydney": "127.53", "Central": "195.39", "Frankfurt": "187.81", "Stockholm": "209.0", "Milan": "179.38", "Ireland": "213.79", "London": "204.32", "Paris": "197.86", "Bahrain": "128.52", "Sao Paulo": "306.81", "N. Virginia": "196.58", "Ohio": "183.07", "N. California": "155.83", "Oregon": "145.4"}, "Tokyo": {"Cape Town": "363.38", "Hong Kong": "54.67", "Tokyo": "6.28", "Seoul": "34.88", "Osaka": "14.4", "Mumbai": "130.81", "Singapore": "76.79", "Sydney": "110.15", "Central": "144.41", "Frankfurt": "226.84", "Stockholm": "245.93", "Milan": "214.97", "Ireland": "202.59", "London": "211.73", "Paris": "217.9", "Bahrain": "168.55", "Sao Paulo": "256.61", "N. Virginia": "147.88", "Ohio": "132.09", "N. California": "110.6", "Oregon": "100.55"}, "Seoul": {"Cape Town": "389.61", "Hong Kong": "38.7", "Tokyo": "34.3", "Seoul": "2.63", "Osaka": "31.42", "Mumbai": "125.13", "Singapore": "71.77", "Sydney": "146.15", "Central": "172.82", "Frankfurt": "223.4", "Stockholm": "246.09", "Milan": "214.75", "Ireland": "230.11", "London": "242.02", "Paris": "244.82", "Bahrain": "162.89", "Sao Paulo": "285.14", "N. Virginia": "174.61", "Ohio": "160.65", "N. California": "136.87", "Oregon": "127.16"}, "Osaka": {"Cape Town": "365.24", "Hong Kong": "49.43", "Tokyo": "9.6", "Seoul": "29.86", "Osaka": "1.37", "Mumbai": "122.63", "Singapore": "73.5", "Sydney": "117.09", "Central": "149.27", "Frankfurt": "228.45", "Stockholm": "249.56", "Milan": "218.94", "Ireland": "207.43", "London": "216.38", "Paris": "222.28", "Bahrain": "164.6", "Sao Paulo": "262.45", "N. Virginia": "151.66", "Ohio": "138.2", "N. California": "108.94", "Oregon": "99.57"}, "Mumbai": {"Cape Town": "223.27", "Hong Kong": "93.12", "Tokyo": "130.12", "Seoul": "125.28", "Osaka": "127.65", "Mumbai": "4.78", "Singapore": "60.44", "Sydney": "153.33", "Central": "188.97", "Frankfurt": "117.2", "Stockholm": "136.36", "Milan": "110.34", "Ireland": "124.67", "London": "114.35", "Paris": "106.6", "Bahrain": "42.1", "Sao Paulo": "299.05", "N. Virginia": "187.54", "Ohio": "198.76", "N. California": "231.53", "Oregon": "224.66"}, "Singapore": {"Cape Town": "272.52", "Hong Kong": "37.87", "Tokyo": "72.94", "Seoul": "72.48", "Osaka": "72.14", "Mumbai": "60.11", "Singapore": "3.01", "Sydney": "96.13", "Central": "212.25", "Frankfurt": "157.9", "Stockholm": "177.94", "Milan": "148.15", "Ireland": "182.6", "London": "172.57", "Paris": "167.25", "Bahrain": "97.59", "Sao Paulo": "326.28", "N. Virginia": "215.85", "Ohio": "201.38", "N. California": "178.71", "Oregon": "167.8"}, "Sydney": {"Cape Town": "416.64", "Hong Kong": "128.97", "Tokyo": "110.63", "Seoul": "145.84", "Osaka": "116.95", "Mumbai": "150.87", "Singapore": "94.94", "Sydney": "2.07", "Central": "199.57", "Frankfurt": "252.12", "Stockholm": "293.82", "Milan": "240.82", "Ireland": "257.33", "London": "268.11", "Paris": "279.75", "Bahrain": "190.51", "Sao Paulo": "312.54", "N. Virginia": "197.68", "Ohio": "187.15", "N. California": "139.07", "Oregon": "141.98"}, "Central": {"Cape Town": "229.74", "Hong Kong": "193.61", "Tokyo": "143.66", "Seoul": "171.92", "Osaka": "149.88", "Mumbai": "189.08", "Singapore": "211.18", "Sydney": "197.75", "Central": "2.87", "Frankfurt": "91.71", "Stockholm": "106.78", "Milan": "101.26", "Ireland": "71.54", "London": "78.58", "Paris": "83.75", "Bahrain": "159.48", "Sao Paulo": "124.81", "N. Virginia": "14.67", "Ohio": "24.55", "N. California": "79.02", "Oregon": "59.86"}, "Frankfurt": {"Cape Town": "160.65", "Hong Kong": "191.81", "Tokyo": "225.15", "Seoul": "224.88", "Osaka": "231.03", "Mumbai": "116.6", "Singapore": "160.15", "Sydney": "250.78", "Central": "93.12", "Frankfurt": "3.76", "Stockholm": "21.93", "Milan": "13.28", "Ireland": "26.28", "London": "17.57", "Paris": "11.29", "Bahrain": "88.49", "Sao Paulo": "203.51", "N. Virginia": "91.8", "Ohio": "104.64", "N. California": "150.42", "Oregon": "144.27"}, "Stockholm": {"Cape Town": "182.55", "Hong Kong": "209.78", "Tokyo": "244.75", "Seoul": "243.65", "Osaka": "248.91", "Mumbai": "137.87", "Singapore": "180.62", "Sydney": "294.98", "Central": "109.86", "Frankfurt": "24.06", "Stockholm": "2.38", "Milan": "32.16", "Ireland": "43.54", "London": "32.44", "Paris": "31.16", "Bahrain": "107.09", "Sao Paulo": "215.15", "N. Virginia": "111.77", "Ohio": "121.12", "N. California": "174.44", "Oregon": "157.71"}, "Milan": {"Cape Town": "168.5", "Hong Kong": "179.68", "Tokyo": "214.26", "Seoul": "213.66", "Osaka": "219.35", "Mumbai": "110.22", "Singapore": "149.53", "Sydney": "241.46", "Central": "100.64", "Frankfurt": "11.29", "Stockholm": "30.46", "Milan": "2.02", "Ireland": "35.02", "London": "25.67", "Paris": "20.16", "Bahrain": "89.34", "Sao Paulo": "212.89", "N. Virginia": "100.55", "Ohio": "110.59", "N. California": "160.48", "Oregon": "150.52"}, "Ireland": {"Cape Town": "169.41", "Hong Kong": "213.42", "Tokyo": "202.69", "Seoul": "232.24", "Osaka": "209.01", "Mumbai": "124.94", "Singapore": "181.66", "Sydney": "257.92", "Central": "72.09", "Frankfurt": "28.68", "Stockholm": "43.11", "Milan": "35.94", "Ireland": "2.39", "London": "14.54", "Paris": "20.07", "Bahrain": "97.76", "Sao Paulo": "177.08", "N. Virginia": "71.0", "Ohio": "78.72", "N. California": "139.34", "Oregon": "119.15"}, "London": {"Cape Town": "154.82", "Hong Kong": "204.0", "Tokyo": "211.94", "Seoul": "242.37", "Osaka": "215.62", "Mumbai": "114.77", "Singapore": "173.55", "Sydney": "265.41", "Central": "79.02", "Frankfurt": "19.29", "Stockholm": "32.65", "Milan": "25.72", "Ireland": "13.8", "London": "2.36", "Paris": "9.67", "Bahrain": "85.44", "Sao Paulo": "186.28", "N. Virginia": "77.16", "Ohio": "86.13", "N. California": "148.13", "Oregon": "128.34"}, "Paris": {"Cape Town": "150.18", "Hong Kong": "198.04", "Tokyo": "216.2", "Seoul": "245.57", "Osaka": "221.54", "Mumbai": "106.92", "Singapore": "165.78", "Sydney": "278.85", "Central": "86.32", "Frankfurt": "13.03", "Stockholm": "31.26", "Milan": "21.55", "Ireland": "18.51", "London": "10.65", "Paris": "2.12", "Bahrain": "81.28", "Sao Paulo": "195.4", "N. Virginia": "83.39", "Ohio": "92.99", "N. California": "142.45", "Oregon": "134.86"}, "Bahrain": {"Cape Town": "246.18", "Hong Kong": "128.59", "Tokyo": "169.23", "Seoul": "162.73", "Osaka": "164.96", "Mumbai": "41.28", "Singapore": "97.19", "Sydney": "186.88", "Central": "162.03", "Frankfurt": "87.44", "Stockholm": "103.98", "Milan": "83.55", "Ireland": "95.35", "London": "86.4", "Paris": "78.55", "Bahrain": "3.96", "Sao Paulo": "272.02", "N. Virginia": "160.3", "Ohio": "169.18", "N. California": "219.1", "Oregon": "210.34"}, "Sao Paulo": {"Cape Town": "343.84", "Hong Kong": "307.42", "Tokyo": "257.42", "Seoul": "286.75", "Osaka": "262.87", "Mumbai": "300.95", "Singapore": "326.64", "Sydney": "311.9", "Central": "126.1", "Frankfurt": "204.08", "Stockholm": "216.82", "Milan": "215.2", "Ireland": "178.7", "London": "187.72", "Paris": "196.48", "Bahrain": "272.56", "Sao Paulo": "3.49", "N. Virginia": "114.64", "Ohio": "125.22", "N. California": "173.95", "Oregon": "175.98"}, "N. Virginia": {"Cape Town": "234.73", "Hong Kong": "195.96", "Tokyo": "145.35", "Seoul": "174.24", "Osaka": "153.32", "Mumbai": "190.23", "Singapore": "215.86", "Sydney": "197.73", "Central": "16.7", "Frankfurt": "91.76", "Stockholm": "111.43", "Milan": "101.05", "Ireland": "67.51", "London": "78.45", "Paris": "82.66", "Bahrain": "158.9", "Sao Paulo": "113.95", "N. Virginia": "3.88", "Ohio": "16.47", "N. California": "64.09", "Oregon": "63.33"}, "Ohio": {"Cape Town": "240.91", "Hong Kong": "184.09", "Tokyo": "132.45", "Seoul": "160.94", "Osaka": "138.05", "Mumbai": "197.49", "Singapore": "203.26", "Sydney": "187.4", "Central": "30.68", "Frankfurt": "105.07", "Stockholm": "121.67", "Milan": "114.01", "Ireland": "79.28", "London": "90.32", "Paris": "93.3", "Bahrain": "169.33", "Sao Paulo": "124.21", "N. Virginia": "15.1", "Ohio": "6.69", "N. California": "55.26", "Oregon": "56.15"}, "N. California": {"Cape Town": "292.5", "Hong Kong": "155.15", "Tokyo": "108.21", "Seoul": "137.14", "Osaka": "109.99", "Mumbai": "230.64", "Singapore": "177.77", "Sydney": "139.92", "Central": "79.66", "Frankfurt": "151.03", "Stockholm": "174.14", "Milan": "160.7", "Ireland": "137.91", "London": "147.06", "Paris": "142.84", "Bahrain": "218.93", "Sao Paulo": "172.85", "N. Virginia": "60.94", "Ohio": "50.92", "N. California": "2.66", "Oregon": "22.84"}, "Oregon": {"Cape Town": "276.3", "Hong Kong": "147.24", "Tokyo": "97.97", "Seoul": "126.75", "Osaka": "99.05", "Mumbai": "222.88", "Singapore": "168.23", "Sydney": "140.91", "Central": "60.2", "Frankfurt": "141.27", "Stockholm": "157.11", "Milan": "150.71", "Ireland": "119.15", "London": "128.46", "Paris": "134.28", "Bahrain": "210.45", "Sao Paulo": "175.06", "N. Virginia": "63.32", "Ohio": "48.68", "N. California": "22.05", "Oregon": "2.62"}}`

// Run go generate from this directory to generate the latency matrix.
//
//go:generate go run latency_gen.go -dest ../../backend/latencies.go
func main() {
	dstFile := flag.String("dest", "", "Destination path and file to save latency map file to.")
	flag.Parse()
	if *dstFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	var allToAllMatrix map[string]map[string]string
	if err := json.Unmarshal([]byte(latencyMatrix), &allToAllMatrix); err != nil {
		log.Fatal(err)
	}
	keys := maps.Keys(allToAllMatrix)
	sort.Strings(keys)

	s := strings.Builder{}
	s.WriteString(`package backend

import "time"

var latencies = map[string]map[string]time.Duration{
`)
	for _, city := range keys {
		s.WriteString(fmt.Sprintf("%q: {\n", city))
		latencies := allToAllMatrix[city]
		for _, city2 := range keys {
			latency, err := time.ParseDuration(latencies[city2] + "ms")
			if err != nil {
				log.Fatal(err)
			}
			s.WriteString(fmt.Sprintf("%q: %d,\n", city2, int64(latency)))
		}
		s.WriteString(fmt.Sprintln("},"))
	}
	s.WriteString(fmt.Sprintln("}"))
	latenciesGoCode, err := format.Source([]byte(s.String()))
	if err != nil {
		log.Fatal(err)
	}
	err = os.WriteFile(*dstFile, latenciesGoCode, 0o600)
	if err != nil {
		log.Fatal(err)
	}
}
