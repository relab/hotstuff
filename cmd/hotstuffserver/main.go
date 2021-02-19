package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/config"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/internal/cli"
	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/profiling"
	"github.com/spf13/pflag"
	"google.golang.org/grpc/credentials"
)

type options struct {
	RootCAs         []string `mapstructure:"root-cas"`
	Privkey         string
	Cert            string
	SelfID          hotstuff.ID `mapstructure:"self-id"`
	PmType          string      `mapstructure:"pacemaker"`
	LeaderID        hotstuff.ID `mapstructure:"leader-id"`
	ViewTimeout     int         `mapstructure:"view-timeout"`
	BatchSize       int         `mapstructure:"batch-size"`
	PrintThroughput bool        `mapstructure:"print-throughput"`
	PrintCommands   bool        `mapstructure:"print-commands"`
	ClientAddr      string      `mapstructure:"client-listen"`
	PeerAddr        string      `mapstructure:"peer-listen"`
	TLS             bool
	Interval        int
	Output          string
	Replicas        []struct {
		ID         hotstuff.ID
		PeerAddr   string `mapstructure:"peer-address"`
		ClientAddr string `mapstructure:"client-address"`
		Pubkey     string
		Cert       string
	}
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

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// some configuration options can be set using flags
	help := pflag.BoolP("help", "h", false, "Prints this text.")
	configFile := pflag.String("config", "", "The path to the config file")
	cpuprofile := pflag.String("cpuprofile", "", "File to write CPU profile to")
	memprofile := pflag.String("memprofile", "", "File to write memory profile to")
	fullprofile := pflag.String("fullprofile", "", "File to write fgprof profile to")
	traceFile := pflag.String("trace", "", "File to write execution trace to")
	pflag.Uint32("self-id", 0, "The id for this replica.")
	pflag.Int("view-change", 100, "How many views before leader change with round-robin pacemaker")
	pflag.Int("batch-size", 100, "How many commands are batched together for each proposal")
	pflag.Int("view-timeout", 1000, "How many milliseconds before a view is timed out")
	pflag.String("privkey", "", "The path to the private key file")
	pflag.String("cert", "", "Path to the certificate")
	pflag.Bool("print-commands", false, "Commands will be printed to stdout")
	pflag.Bool("print-throughput", false, "Throughput measurements will be printed stdout")
	pflag.Int("interval", 1000, "Throughput measurement interval in milliseconds")
	pflag.Bool("tls", false, "Enable TLS")
	pflag.String("client-listen", "", "Override the listen address for the client server")
	pflag.String("peer-listen", "", "Override the listen address for the replica (peer) server")
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

	// TODO: replace with go 1.16 signal.NotifyContext
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-signals
		fmt.Fprintf(os.Stderr, "Exiting...")
		cancel()
	}()

	start(ctx, &conf)
}

func start(ctx context.Context, conf *options) {
	privkey, err := crypto.ReadPrivateKeyFile(conf.Privkey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read private key file: %v\n", err)
		os.Exit(1)
	}

	var creds credentials.TransportCredentials
	var tlsCert tls.Certificate
	if conf.TLS {
		creds, tlsCert = loadCreds(conf)
	}

	var clientAddress string
	replicaConfig := config.NewConfig(conf.SelfID, privkey, creds)
	for _, r := range conf.Replicas {
		key, err := crypto.ReadPublicKeyFile(r.Pubkey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read public key file '%s': %v\n", r.Pubkey, err)
			os.Exit(1)
		}

		info := &config.ReplicaInfo{
			ID:      r.ID,
			Address: r.PeerAddr,
			PubKey:  key,
		}

		if r.ID == conf.SelfID {
			// override own addresses if set
			if conf.ClientAddr != "" {
				clientAddress = conf.ClientAddr
			} else {
				clientAddress = r.ClientAddr
			}
			if conf.PeerAddr != "" {
				info.Address = conf.PeerAddr
			}
		}

		replicaConfig.Replicas[r.ID] = info
	}

	logging.NameLogger(fmt.Sprintf("hs%d", conf.SelfID))

	srv := newClientServer(conf, replicaConfig, &tlsCert)
	err = srv.Start(clientAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start HotStuff: %v\n", err)
		os.Exit(1)
	}

	<-ctx.Done()
	srv.Stop()
}

func loadCreds(conf *options) (credentials.TransportCredentials, tls.Certificate) {
	if conf.Cert == "" {
		for _, replica := range conf.Replicas {
			if replica.ID == conf.SelfID {
				conf.Cert = replica.Cert
			}
		}
	}

	tlsCert, err := tls.LoadX509KeyPair(conf.Cert, conf.Privkey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse certificate: %v\n", err)
		os.Exit(1)
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get system cert pool: %v\n", err)
		os.Exit(1)
	}

	for _, ca := range conf.RootCAs {
		cert, err := os.ReadFile(ca)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read CA file: %v\n", err)
			os.Exit(1)
		}
		if !rootCAs.AppendCertsFromPEM(cert) {
			fmt.Fprintf(os.Stderr, "Failed to add CA to cert pool.\n")
			os.Exit(1)
		}
	}
	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      rootCAs,
		ClientCAs:    rootCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	})

	return creds, tlsCert
}
