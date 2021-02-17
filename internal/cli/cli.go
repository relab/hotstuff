package cli

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ReadConfig reads config options from configuration files and command line flags.
func ReadConfig(opts interface{}, secondaryConfig string) (err error) {
	err = viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		return err
	}

	// read main config file in working dir
	viper.SetConfigName("hotstuff")
	viper.AddConfigPath(".")
	err = viper.ReadInConfig()
	if err != nil {
		return err
	}

	// read secondary config if requested
	if secondaryConfig != "" {
		viper.SetConfigFile(secondaryConfig)
		err = viper.MergeInConfig()
		if err != nil {
			return err
		}
	}

	err = viper.Unmarshal(opts)
	if err != nil {
		return err
	}

	return nil
}
