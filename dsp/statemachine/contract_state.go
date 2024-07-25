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

package statemachine

type contractState int

//go:generate goenums contract_state.go
const (
	initial    contractState = iota // INITIAL
	requested                       // dspace:REQUESTED
	offered                         // dspace:OFFERED
	agreed                          // dspace:AGREED
	accepted                        // dspace:ACCEPTED
	verified                        // dspace:VERIFIED
	finalized                       // dspace:FINALIZED
	terminated                      // dspace:TERMINATED
)
