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

package main

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/go-dataspace/run-dsp/internal/cli"
	"github.com/go-dataspace/run-dsp/internal/server"
)

var ui struct {
	cli.GlobalOptions
	Server server.Command `cmd:"" help:"Run server"`
}

func main() {
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "--help")
	}

	ctx := kong.Parse(&ui)
	params := cli.GenParams(ui.GlobalOptions)
	ctx.BindTo(params, (*cli.Params)(nil))
	ctx.FatalIfErrorf(ctx.Run())
}
