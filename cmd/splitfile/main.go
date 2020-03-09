package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	flag "github.com/spf13/pflag"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] filename target\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "target should be the target file with a '*' that will be replaced by a number.\n\n")
	fmt.Fprintf(os.Stderr, "OPTIONS:\n")
	flag.PrintDefaults()
}

func writeChunks(wg *sync.WaitGroup, c chan []byte, filename string) {
	wg.Add(1)
	defer wg.Done()

	file, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create file '%s': %v\n", filename, err)
		os.Exit(1)
	}
	defer file.Close()

	for b := range c {
		_, err := file.Write(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write to file '%s': %v\n", filename, err)
			os.Exit(1)
		}
	}

	fmt.Printf("Wrote file '%s'.\n", filename)
}

func main() {
	numFiles := flag.IntP("num-files", "n", 4, "Number of files to split into (default 4).")
	size := flag.IntP("chunk-size", "s", 1024, "Chunk size in bytes (Default 1024)")
	help := flag.BoolP("help", "h", false, "Prints this message")
	flag.Parse()

	if *help {
		usage()
		os.Exit(0)
	}

	filename := flag.Arg(0)
	target := flag.Arg(1)

	if target == "" {
		fmt.Fprintf(os.Stderr, "info: target missing. Will create files next to '%s'.\n", filename)
		target = fmt.Sprintf("%s.*", filename)
	}

	if strings.IndexByte(target, '*') == -1 {
		fmt.Fprintf(os.Stderr, "info: target missing '*'. Will append number.\n")
		target = fmt.Sprintf("%s.*", target)
	}

	file, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open file '%s': %v\n", filename, err)
		os.Exit(1)
	}
	defer file.Close()

	var wg sync.WaitGroup
	chans := make([]chan []byte, *numFiles)
	for i := 0; i < *numFiles; i++ {
		chans[i] = make(chan []byte)
		go writeChunks(&wg, chans[i], strings.ReplaceAll(target, "*", strconv.Itoa(i+1)))
	}

	buf := make([]byte, *size)
	i := 0
	for {
		n, err := file.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "Failed to read from file '%s': %v\n", filename, err)
				os.Exit(1)
			}
			break
		}

		b := make([]byte, n)
		copy(b, buf[:n])
		chans[i] <- b

		i = (i + 1) % *numFiles
	}

	for _, c := range chans {
		close(c)
	}

	wg.Wait()
}
