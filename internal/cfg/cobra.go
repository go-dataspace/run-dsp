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

// Package cfg contains configuration helpers for RUN-DSP.
package cfg

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// AddPersistentFlag adds a persistent flag `flag“, with default `def“ and usage message `usage`,
// and it will also bind the flag to the viper `configKey`.
func AddPersistentFlag(cmd *cobra.Command, configKey, flag, usage string, def any) {
	switch v := def.(type) {
	case int:
		cmd.PersistentFlags().Int(flag, v, usage)
	case string:
		cmd.PersistentFlags().String(flag, v, usage)
	case bool:
		cmd.PersistentFlags().Bool(flag, v, usage)
	default:
		panic(fmt.Sprintf("Unsupported type: %T", v))
	}
	err := viper.BindPFlag(configKey, cmd.PersistentFlags().Lookup(flag))
	if err != nil {
		// impossible in this setup
		panic(err.Error())
	}
	viper.SetDefault(configKey, def)
}
