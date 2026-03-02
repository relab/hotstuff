// Package cli provide command line interface helpers for hotstuff.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/rules/byzantine"
	"github.com/relab/hotstuff/security/crypto"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rootCmd represents the base command when called without any subcommands
var (
	listModules bool
	cfgFile     string

	rootCmd = &cobra.Command{
		Use:   "hotstuff",
		Short: "A command-line utility for running experiments.",
		Long: `hotstuff is a command-line utility for performing experiments with consensus protocols.
It can run experiments locally, or on multiple nodes via SSH.
hotstuff can automatically assign replicas and clients to be run on each Node,
run the experiment, and fetch the experiment results.

To run an experiment, use the 'hotstuff run' command.
By default, this command will run a small configuration of replicas locally.
use 'hotstuff help run' to view all possible parameters for this command.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !listModules {
				return cmd.Usage()
			}
			modules := map[string][]string{
				"--consensus": {
					rules.NameSimpleHotStuff,
					rules.NameFastHotStuff,
					rules.NameChainedHotStuff,
				},
				"--byzantine-strategy": {
					byzantine.NameSilentProposer,
					byzantine.NameFork,
					byzantine.NameIncreaseView,
				},
				"--crypto": {
					crypto.NameECDSA,
					crypto.NameEDDSA,
					crypto.NameBLS12,
				},
				"--leader-rotation": {
					leaderrotation.NameRoundRobin,
					leaderrotation.NameCarousel,
					leaderrotation.NameFixed,
					leaderrotation.NameTree,
					leaderrotation.NameReputation,
				},
				"--communication": {
					comm.NameClique,
					comm.NameKauri,
				},
			}
			mods := modules
			for k, v := range mods {
				fmt.Println(k, ":")
				for _, n := range v {
					fmt.Println("\t", n)
				}
			}
			return nil
		},
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.Flags().BoolVar(&listModules, "list-modules", false, "list available modules")

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.hotstuff.yaml)")

	rootCmd.PersistentFlags().String("log-level", "info", "sets the log level (debug, info, warn, error")
	cobra.CheckErr(viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")))
	rootCmd.PersistentFlags().StringSlice("log-pkgs", []string{}, "set the log level on a per-package basis.")
	cobra.CheckErr(viper.BindPFlag("log-pkgs", rootCmd.PersistentFlags().Lookup("log-pkgs")))

	// Loki log aggregation flags
	rootCmd.PersistentFlags().String("loki-url", "", "Loki push API URL (e.g. http://localhost:3100/loki/api/v1/push). Enables Loki log shipping when set.")
	cobra.CheckErr(viper.BindPFlag("loki-url", rootCmd.PersistentFlags().Lookup("loki-url")))
	rootCmd.PersistentFlags().StringSlice("loki-labels", []string{}, "static labels for Loki streams as key=value pairs (e.g. node=replica-1,env=dev)")
	cobra.CheckErr(viper.BindPFlag("loki-labels", rootCmd.PersistentFlags().Lookup("loki-labels")))
	rootCmd.PersistentFlags().String("loki-tenant-id", "", "Loki tenant ID for multi-tenant setups (X-Scope-OrgID header)")
	cobra.CheckErr(viper.BindPFlag("loki-tenant-id", rootCmd.PersistentFlags().Lookup("loki-tenant-id")))
	rootCmd.PersistentFlags().Int("loki-batch-size", 100, "max log entries to buffer before pushing to Loki")
	cobra.CheckErr(viper.BindPFlag("loki-batch-size", rootCmd.PersistentFlags().Lookup("loki-batch-size")))
	rootCmd.PersistentFlags().Duration("loki-batch-wait", 1*time.Second, "max time to wait before flushing a partial batch to Loki")
	cobra.CheckErr(viper.BindPFlag("loki-batch-wait", rootCmd.PersistentFlags().Lookup("loki-batch-wait")))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// TODO(meling): We should use the OS's default config directory instead; os.UserConfigDir().
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".hotstuff" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".hotstuff")
	}

	viper.SetEnvPrefix("hotstuff")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	logging.SetLogLevel(viper.GetString("log-level"))

	// Configure Loki if URL is provided (via --loki-url flag or HOTSTUFF_LOKI_URL env var)
	lokiURL := viper.GetString("loki-url")
	if lokiURL != "" {
		labels := map[string]string{"app": "hotstuff"}
		for _, lbl := range viper.GetStringSlice("loki-labels") {
			parts := strings.SplitN(lbl, "=", 2)
			if len(parts) == 2 {
				labels[parts[0]] = parts[1]
			}
		}
		logging.SetLokiConfig(&logging.LokiConfig{
			URL:       lokiURL,
			Labels:    labels,
			TenantID:  viper.GetString("loki-tenant-id"),
			BatchSize: viper.GetInt("loki-batch-size"),
			BatchWait: viper.GetDuration("loki-batch-wait"),
		})
	}

	packageLevels := viper.GetStringSlice("log-pkgs")

	for _, packageLevel := range packageLevels {
		parts := strings.Split(packageLevel, ":")
		if len(parts) != 2 {
			fmt.Println("log-pkgs flag must be a comma-separated list of package:level strings")
			os.Exit(1)
		}
		logging.SetPackageLogLevel(parts[0], parts[1])
	}
}
