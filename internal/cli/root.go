package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/spf13/cobra"

	homedir "github.com/mitchellh/go-homedir"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if !listModules {
				return cmd.Usage()
			}
			mods := modules.ListModules()
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
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
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
