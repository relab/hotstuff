package cmd

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/relab/hotstuff/internal/logging"
	"github.com/relab/hotstuff/internal/orchestration"
	"github.com/relab/hotstuff/internal/protostream"
	"github.com/relab/iago"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an experiment.",
	Long:  `The run command runs an experiment locally or on remote workers.`,
	Run: func(cmd *cobra.Command, args []string) {
		runController()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().Int("replicas", 4, "number of replicas to run")
	runCmd.Flags().Int("clients", 1, "number of clients to run")
	runCmd.Flags().Int("batch-size", 1, "number of commands to batch together in each block")
	runCmd.Flags().Int("payload-size", 0, "size in bytes of the command payload")
	runCmd.Flags().Int("max-concurrent", 4, "maximum number of conccurrent commands per client")
	runCmd.Flags().Duration("duration", 10*time.Second, "duration of the experiment")
	runCmd.Flags().Duration("connect-timeout", 5*time.Second, "duration of the initial connection timeout")
	runCmd.Flags().Duration("view-timeout", 100*time.Millisecond, "duration of the first view")
	runCmd.Flags().Int("duration-samples", 1000, "number of previous views to consider when predicting view duration")
	runCmd.Flags().Float32("timeout-multiplier", 1.2, "number to multiply the view duration by in case of a timeout")
	runCmd.Flags().String("consensus", "chainedhotstuff", "name of the consensus implementation")
	runCmd.Flags().String("crypto", "ecdsa", "name of the crypto implementation")
	runCmd.Flags().String("leader-rotation", "round-robin", "name of the leader rotation algorithm")

	runCmd.Flags().Bool("worker", false, "run a local worker")
	runCmd.Flags().StringSlice("hosts", nil, "the remote hosts to run the experiment on via ssh")
	runCmd.Flags().String("exe", "", "path to the executable to deploy and run on remote workers")
	runCmd.Flags().String("ssh-config", "", "path to ssh_config file to resolve host aliases (defaults to ~/.ssh/config)")

	err := viper.BindPFlags(runCmd.Flags())
	if err != nil {
		panic(err)
	}
}

func runController() {
	experiment := orchestration.Experiment{
		NumReplicas:       viper.GetInt("replicas"),
		NumClients:        viper.GetInt("clients"),
		BatchSize:         viper.GetInt("batch-size"),
		PayloadSize:       viper.GetInt("payload-size"),
		MaxConcurrent:     viper.GetInt("max-concurrent"),
		Duration:          viper.GetDuration("duration"),
		ConnectTimeout:    viper.GetDuration("connect-timeout"),
		ViewTimeout:       viper.GetDuration("view-timeout"),
		TimoutSamples:     viper.GetInt("duration-samples"),
		TimeoutMultiplier: float32(viper.GetFloat64("timeout-multiplier")),
		Consensus:         viper.GetString("consensus"),
		Crypto:            viper.GetString("crypto"),
		LeaderRotation:    viper.GetString("leader-rotation"),
	}

	log := logging.New("ctrl")

	worker := viper.GetBool("worker")
	hosts := viper.GetStringSlice("hosts")
	exePath := viper.GetString("exe")

	g, err := iago.NewSSHGroup(hosts, viper.GetString("ssh-config"))
	if err != nil {
		log.Fatal("Failed to connect to remote hosts: ", err)
	}

	if exePath == "" {
		exePath, err = os.Executable()
		if err != nil {
			log.Fatal("Failed to get executable path: ", err)
		}
	}

	sessions, err := orchestration.Deploy(g, exePath, viper.GetString("log-level"))
	if err != nil {
		log.Fatal("Failed to deploy workers: ", err)
	}

	errors := make(chan error)

	experiment.Hosts = make(map[string]orchestration.RemoteWorker)

	for host, session := range sessions {
		experiment.Hosts[host] = orchestration.NewRemoteWorker(
			protostream.NewWriter(session.Stdin()), protostream.NewReader(session.Stdout()),
		)
		go stderrPipe(session.Stderr(), errors)
	}

	if worker {
		experiment.Hosts["localhost"] = localWorker()
	}

	experiment.HostConfigs = make(map[string]orchestration.HostConfig)

	var hostConfigs []struct {
		Name     string
		Clients  int
		Replicas int
	}

	err = viper.UnmarshalKey("hosts-config", &hostConfigs)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to unmarshal hosts-config: %w", err))
	}

	for _, cfg := range hostConfigs {
		experiment.HostConfigs[cfg.Name] = orchestration.HostConfig{Replicas: cfg.Replicas, Clients: cfg.Clients}
	}

	err = experiment.Run()
	if err != nil {
		log.Fatal(err)
	}

	for _, session := range sessions {
		err := session.Close()
		if err != nil {
			log.Fatal(err)
		}
	}

	for range sessions {
		err = <-errors
		if err != nil {
			log.Fatal(err)
		}
	}

	err = g.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func localWorker() orchestration.RemoteWorker {
	// set up a local worker
	controllerPipe, workerPipe := net.Pipe()
	go func() {
		worker := orchestration.NewWorker(protostream.NewWriter(workerPipe), protostream.NewReader(workerPipe))
		err := worker.Run()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return orchestration.NewRemoteWorker(protostream.NewWriter(controllerPipe), protostream.NewReader(controllerPipe))
}

func stderrPipe(r io.Reader, errChan chan<- error) {
	_, err := io.Copy(os.Stderr, r)
	errChan <- err
}
