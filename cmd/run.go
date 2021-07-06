package cmd

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/relab/hotstuff/internal/orchestration"
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
	runCmd.Flags().Duration("duration", 5*time.Second, "duration of the experiment")
	runCmd.Flags().Duration("connect-timeout", time.Second, "duration of the initial connection timeout")
	runCmd.Flags().Duration("view-timeout", time.Second, "duration of the first view")
	runCmd.Flags().Int("duration-samples", 1000, "number of previous views to consider when predicting view duration")
	runCmd.Flags().Float32("timeout-multiplier", 1.2, "number to multiply the view duration by in case of a timeout")
	runCmd.Flags().String("consensus", "chainedhotstuff", "name of the consensus implementation")
	runCmd.Flags().String("crypto", "ecdsa", "name of the crypto implementation")
	runCmd.Flags().String("leader-rotation", "round-robin", "name of the leader rotation algorithm")

	runCmd.Flags().Int("port", 4000, "the port to start remote workers on")
	runCmd.Flags().Bool("worker", false, "run a local worker")
	runCmd.Flags().StringSlice("hosts", nil, "the remote hosts to run the experiment on via ssh")
	runCmd.Flags().String("exe", "", "path to the executable to deploy and run on remote workers")
	runCmd.Flags().String("ssh-config", "", "path to ssh_config file to resolve host aliases (defaults to ~/.ssh/config)")

	viper.BindPFlags(runCmd.Flags())
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

	remotePort := viper.GetInt("port")
	worker := viper.GetBool("worker")
	hosts := viper.GetStringSlice("hosts")
	exePath := viper.GetString("exe")

	g, err := iago.NewSSHGroup(hosts, viper.GetString("ssh-config"))
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

	// append the port to each hostname
	hostsPorts := make([]string, 0)
	for _, h := range hosts {
		hostsPorts = append(hostsPorts, fmt.Sprintf("%s:%d", h, remotePort))
	}

	if worker {
		go runWorker(remotePort)
		hostsPorts = append(hostsPorts, fmt.Sprintf("localhost:%d", remotePort))
	}

	err = experiment.Run(hostsPorts)
	if err != nil {
		log.Fatal(err)
	}
}
