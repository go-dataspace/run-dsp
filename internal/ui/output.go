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

// Package ui contains UI functions, mostly output functions at this time.
package ui

import (
	"os"

	"github.com/fatih/color"
)

// Error prints a red error message to stderr.
func Error(message string) {
	color.New(color.BgRed, color.FgWhite, color.Bold).Fprint(os.Stderr, " ERROR ")
	color.New(color.FgRed, color.Bold).Fprintln(os.Stderr, " "+message)
}

// Warn prints a yellow warning message to stderr.
func Warn(message string) {
	color.New(color.BgYellow, color.FgWhite, color.Bold).Fprint(os.Stderr, " WARN ")
	color.New(color.FgYellow, color.Bold).Fprintln(os.Stderr, " "+message)
}

// Info prints a green informational message to stderr.
func Info(message string) {
	color.New(color.BgGreen, color.FgWhite, color.Bold).Fprint(os.Stderr, " INFO ")
	color.New(color.FgGreen, color.Bold).Fprintln(os.Stderr, " "+message)
}

// Print is a normal message, to stdout.
func Print(message string) {
	color.New().Print(message)
}
