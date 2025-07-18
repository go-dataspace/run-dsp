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
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/internal/server"
	"go-dataspace.eu/run-dsp/internal/ui"
	"go-dataspace.eu/run-dsp/logging"
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

			ctx := ctxslog.New(logging.New(logLevel, viper.GetBool("humanReadable")))
			viper.Set("initCTX", ctx)
			return nil
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	cobra.EnableTraverseRunHooks = true

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is /etc/run-dsp/run-dsp.toml)")
	rootCmd.PersistentFlags().BoolP("human-readable", "r", false, "Print human-readable logs (instead of JSON)")
	rootCmd.PersistentFlags().StringP(
		"log-level", "l", "info", fmt.Sprintf("set log level, valid levels: %v", validLogLevels))

	err := viper.BindPFlag("humanReadable", rootCmd.PersistentFlags().Lookup("human-readable"))
	if err != nil {
		panic(err.Error())
	}
	err = viper.BindPFlag("logLevel", rootCmd.PersistentFlags().Lookup("log-level"))
	if err != nil {
		panic(err.Error())
	}

	viper.SetDefault("humanReadable", false)
	viper.SetDefault("logLevel", "info")

	rootCmd.AddCommand(server.Command)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath("/etc/run-dsp")
		viper.AddConfigPath("$HOME/.config/run-dsp")
		viper.SetConfigType("toml")
		viper.SetConfigName("run-dsp.toml")
	}

	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		ui.Print(fmt.Sprintln("Using config file:", viper.ConfigFileUsed()))
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		ui.Error(err.Error())
		os.Exit(1)
	}
}
