package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/cli"
	"github.com/relab/hotstuff/internal/profiling"
	"github.com/spf13/pflag"
)

type replica struct {
	ID         hotstuff.ID
	PeerAddr   string `mapstructure:"peer-address"`
	ClientAddr string `mapstructure:"client-address"`
	Pubkey     string
	Cert       string
}

type options struct {
	BatchSize       int         `mapstructure:"batch-size"`
	Benchmark       bool        `mapstructure:"benchmark"`
	Cert            string      `mapstructure:"cert"`
	CertKey         string      `mapstructure:"cert-key"`
	Crypto          string      `mapstructure:"crypto"`
	Consensus       string      `mapstructure:"consensus"`
	ClientAddr      string      `mapstructure:"client-listen"`
	ExitAfter       int         `mapstructure:"exit-after"`
	Input           string      `mapstructure:"input"`
	LeaderID        hotstuff.ID `mapstructure:"leader-id"`
	MaxInflight     uint64      `mapstructure:"max-inflight"`
	Output          string      `mapstructure:"print-commands"`
	PayloadSize     int         `mapstructure:"payload-size"`
	PeerAddr        string      `mapstructure:"peer-listen"`
	PmType          string      `mapstructure:"pacemaker"`
	PrintThroughput bool        `mapstructure:"print-throughput"`
	Privkey         string
	RateLimit       int         `mapstructure:"rate-limit"`
	RootCAs         []string    `mapstructure:"root-cas"`
	SelfID          hotstuff.ID `mapstructure:"self-id"`
	TLS             bool
	ViewTimeout     int `mapstructure:"view-timeout"`
	Replicas        []replica
}

func usage() {
	fmt.Printf("Usage: %s [options]\n", os.Args[0])
	fmt.Println()
	fmt.Println("Loads configuration from ./hotstuff.toml and file specified by --config")
	fmt.Println()
	fmt.Println("Options:")
	pflag.PrintDefaults()
}

func main() {
	pflag.Usage = usage

	// some configuration options can be set using flags
	help := pflag.BoolP("help", "h", false, "Prints this text.")
	configFile := pflag.String("config", "", "The path to the config file")
	cpuprofile := pflag.String("cpuprofile", "", "File to write CPU profile to")
	memprofile := pflag.String("memprofile", "", "File to write memory profile to")
	fullprofile := pflag.String("fullprofile", "", "File to write fgprof profile to")
	traceFile := pflag.String("trace", "", "File to write execution trace to")
	server := pflag.Bool("server", false, "Start a server. If not specified, a client will be started.")

	// shared options
	pflag.Uint32("self-id", 0, "The id for this replica.")
	pflag.Bool("tls", false, "Enable TLS")
	pflag.Int("exit-after", 0, "Number of seconds after which the program should exit.")

	// server options
	pflag.String("privkey", "", "The path to the private key file (server)")
	pflag.String("cert", "", "Path to the certificate (server)")
	pflag.String("cert-key", "", "Path to the private key for the certificate (server)")
	pflag.String("crypto", "ecdsa", "The name of the crypto implementation to use (server)")
	pflag.String("consensus", "chainedhotstuff", "The name of the consensus implementation to use (server)")
	pflag.Int("view-timeout", 1000, "How many milliseconds before a view is timed out (server)")
	pflag.String("output", "", "Commands will be written here. (server)")
	pflag.Int("batch-size", 100, "How many commands are batched together for each proposal (server)")
	pflag.Bool("print-throughput", false, "Throughput measurements will be printed stdout (server)")
	pflag.String("client-listen", "", "Override the listen address for the client server (server)")
	pflag.String("peer-listen", "", "Override the listen address for the replica (peer) server (server)")

	// client options
	pflag.String("input", "", "Optional file to use for payload data (client)")
	pflag.Bool("benchmark", false, "If enabled, a BenchmarkData protobuf will be written to stdout. (client)")
	pflag.Int("rate-limit", 0, "Limit the request-rate to approximately (in requests per second). (client)")
	pflag.Int("payload-size", 0, "The size of the payload in bytes (client)")
	pflag.Uint64("max-inflight", 10000, "The maximum number of messages that the client can wait for at once (client)")

	pflag.Parse()

	if *help {
		pflag.Usage()
		os.Exit(0)
	}

	profileStop, err := profiling.StartProfilers(*cpuprofile, *memprofile, *traceFile, *fullprofile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start profilers: %v\n", err)
		os.Exit(1)
	}

	defer func() {
		err := profileStop()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop profilers: %v\n", err)
			os.Exit(1)
		}
	}()

	var conf options
	err = cli.ReadConfig(&conf, *configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go func() {
		if conf.ExitAfter > 0 {
			time.Sleep(time.Duration(conf.ExitAfter) * time.Millisecond)
			cancel()
		}
	}()

	if *server {
		runServer(ctx, &conf)
	} else {
		runClient(ctx, &conf)
	}
}
