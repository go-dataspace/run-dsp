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

package dsp

import (
	"slices"

	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/go-playground/validator/v10"
)

func transferProcessState(fl validator.FieldLevel) bool {
	states := []string{
		"dspace:REQUESTED",
		"dspace:STARTED",
		"dspace:TERMINATED",
		"dspace:COMPLETED",
		"dspace:SUSPENDED",
	}
	return slices.Contains(states, fl.Field().String())
}

func contractNegotiationState(fl validator.FieldLevel) bool {
	states := []string{
		"dspace:REQUESTED",
		"dspace:OFFERED",
		"dspace:ACCEPTED",
		"dspace:AGREED",
		"dspace:VERIFIED",
		"dspace:FINALIZED",
		"dspace:TERMINATED",
	}
	return slices.Contains(states, fl.Field().String())
}

// This registers all the validators of this package, and also calls the odrl register function
// as this package uses the odrl structs as well.
func RegisterValidators(v *validator.Validate) error {
	if err := v.RegisterValidation("transfer_state", transferProcessState); err != nil {
		return err
	}
	if err := v.RegisterValidation("contract_state", contractNegotiationState); err != nil {
		return err
	}
	return odrl.RegisterValidators(v)
}
