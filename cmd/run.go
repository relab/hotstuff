package cmd

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Raytar/iago"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// experiment options
	numReplicas    int
	numClients     int
	batchSize      int
	payloadSize    int
	maxConcurrent  int
	duration       int
	connectTimeout int
	consensusName  string
	cryptoName     string
	leaderRotation string

	// worker flags
	remotePort    int
	worker        bool
	hosts         []string
	exePath       string
	sshConfigPath string

	// runCmd represents the run command
	runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run an experiment.",
		Long:  `The run command runs an experiment locally or on remote workers.`,
		Run: func(cmd *cobra.Command, args []string) {
			runController()
		},
	}
)

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().IntVar(&numReplicas, "replicas", 4, "number of replicas to run")
	runCmd.Flags().IntVar(&numClients, "clients", 1, "number of clients to run")
	runCmd.Flags().IntVar(&batchSize, "batch-size", 1, "number of commands to batch together in each block")
	runCmd.Flags().IntVar(&payloadSize, "payload-size", 0, "size in bytes of the command payload")
	runCmd.Flags().IntVar(&maxConcurrent, "max-concurrent", 4, "maximum number of conccurrent commands per client")
	runCmd.Flags().IntVar(&duration, "duration", 5, "duration (in seconds) of the experiment")
	runCmd.Flags().IntVar(&connectTimeout, "connect-timeout", 1000, "duration (in milliseconds) of the initial connection timeout")
	runCmd.Flags().StringVar(&consensusName, "consensus", "chainedhotstuff", "name of the consensus implementation")
	runCmd.Flags().StringVar(&cryptoName, "crypto", "ecdsa", "name of the crypto implementation")
	runCmd.Flags().StringVar(&leaderRotation, "leader-rotation", "round-robin", "name of the leader rotation algorithm")

	runCmd.Flags().IntVar(&remotePort, "port", 4000, "the port to start remote workers on")
	runCmd.Flags().BoolVar(&worker, "worker", false, "run a local worker")
	runCmd.Flags().StringSliceVar(&hosts, "hosts", nil, "the remote hosts to run the experiment on via ssh")
	runCmd.Flags().StringVar(&exePath, "exe", "", "path to the executable to deploy and run on remote workers")
	runCmd.Flags().StringVar(&sshConfigPath, "ssh-config", "", "path to ssh_config file to resolve host aliases (defaults to ~/.ssh/config)")

	viper.BindPFlags(runCmd.Flags())
}

func runController() {
	experiment := orchestration.Experiment{
		NumReplicas:    numReplicas,
		NumClients:     numClients,
		BatchSize:      batchSize,
		PayloadSize:    payloadSize,
		MaxConcurrent:  maxConcurrent,
		Duration:       time.Duration(duration) * time.Second,
		ConnectTimeout: time.Duration(connectTimeout) * time.Millisecond,
		Consensus:      consensusName,
		Crypto:         cryptoName,
		LeaderRotation: leaderRotation,
	}

	g, err := iago.NewSSHGroup(hosts, sshConfigPath)
	if err != nil {
		log.Fatalln("Failed to connect to remote hosts: ", err)
	}

	if exePath == "" {
		exePath, err = os.Executable()
		if err != nil {
			log.Fatalln("Failed to get executable path: ", err)
		}
	}

	err = orchestration.Deploy(g, exePath, strconv.Itoa(remotePort))
	if err != nil {
		log.Fatalln("Failed to deploy workers: ", err)
	}

	if worker {
		go runWorker(remotePort)
		hosts = append(hosts, "localhost:"+strconv.Itoa(remotePort))
	}

	hosts := viper.GetStringSlice("hosts")
	err = experiment.Run(hosts)
	if err != nil {
		log.Fatal(err)
	}
}
