// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package root

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/go-dataspace/run-dsp/internal/server"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string

	validLogLevels = []string{"debug", "info", "warn", "error"}

	rootCmd = &cobra.Command{
		Use:   "run-dsp",
		Short: "RUN-DSP is a lightweight dataspace connector.",
		Long: `A lightweight IDSA dataspace connector, designed to
				connect non-dataspace data providers via gRPC`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logLevel := viper.GetString("logLevel")
			if !slices.Contains(validLogLevels, logLevel) {
				return fmt.Errorf("Invalid log level %s, valid levels: %v", logLevel, validLogLevels)
			}
			ctx := context.Background()
			humanReadable := false
			if viper.GetBool("debug") {
				humanReadable = true
				logLevel = "debug"
			}
			ctx = logging.Inject(ctx, logging.NewJSON(logLevel, humanReadable))
			viper.Set("initCTX", ctx)
			return nil
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	cobra.EnableTraverseRunHooks = true

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is /etc/run-dsp/run-dsp.toml)")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "enable debug mode")
	rootCmd.PersistentFlags().StringP(
		"log-level", "l", "info", fmt.Sprintf("set log level, valid levels: %v", validLogLevels))

	err := viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	if err != nil {
		panic(err.Error())
	}
	err = viper.BindPFlag("logLevel", rootCmd.PersistentFlags().Lookup("log-level"))
	if err != nil {
		panic(err.Error())
	}

	viper.SetDefault("debug", false)
	viper.SetDefault("logLevel", "info")

	rootCmd.AddCommand(server.Command)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath("/etc/run-dsp")
		viper.SetConfigType("toml")
		viper.SetConfigName("run-dsp.toml")
	}

	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		log.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
