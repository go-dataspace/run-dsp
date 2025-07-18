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

package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/go-playground/validator/v10"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/internal/constants"
	"go-dataspace.eu/run-dsp/jsonld"
	"go-dataspace.eu/run-dsp/odrl"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
	if err := RegisterValidators(validate); err != nil {
		panic(err)
	}
}

func TransferProcessState(fl validator.FieldLevel) bool {
	states := []string{
		"INITIAL",
		"dspace:REQUESTED",
		"dspace:STARTED",
		"dspace:TERMINATED",
		"dspace:COMPLETED",
		"dspace:SUSPENDED",
	}
	return slices.Contains(states, fl.Field().String())
}

func ContractNegotiationState(fl validator.FieldLevel) bool {
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

func EncodeValid[T any](w http.ResponseWriter, r *http.Request, status int, v T) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := validate.Struct(v); err != nil {
		return handleValidationError(r.Context(), err)
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func DecodeValid[T any](r *http.Request) (T, error) {
	defer r.Body.Close()
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("decode json: %w", err)
	}

	if err := validate.Struct(v); err != nil {
		return v, handleValidationError(r.Context(), err)
	}

	return v, nil
}

func ValidateAndMarshal[T any](ctx context.Context, s T) ([]byte, error) {
	if err := validate.Struct(s); err != nil {
		err := handleValidationError(ctx, err)
		return nil, err
	}
	return json.Marshal(s)
}

func UnmarshalAndValidate[T any](ctx context.Context, b []byte, s T) (T, error) {
	err := json.Unmarshal(b, &s)
	if err != nil {
		return s, ctxslog.ReturnError(ctx, "Couldn't unmarhal JSON", err)
	}

	if err := validate.Struct(s); err != nil {
		err := handleValidationError(ctx, err)
		return s, err
	}
	return s, nil
}

func handleValidationError(ctx context.Context, err error) error {
	// This should rarely if ever happen, but guard for it anyway.
	if _, ok := err.(*validator.InvalidValidationError); ok { //nolint:errorlint
		return ctxslog.ReturnError(ctx, "Invalid validation", err)
	}

	for _, err := range err.(validator.ValidationErrors) { //nolint:errorlint,forcetypeassert
		ctxslog.Error(ctx,
			"Validation error",
			"Namespace", err.Namespace(),
			"Field", err.Field(),
			"StructNamespace", err.StructNamespace(),
			"Tag", err.Tag(),
			"ActualTag", err.ActualTag(),
			"Kind", err.Kind(),
			"Type", err.Type(),
			"Value", err.Value(),
			"Param", err.Param(),
		)
		return fmt.Errorf("Validation Error: %w", err)
	}
	return ctxslog.ReturnError(ctx, "Unknown error", err)
}

// This registers all the validators of this package, and also calls the odrl register function
// as this package uses the odrl structs as well.
func RegisterValidators(v *validator.Validate) error {
	if err := v.RegisterValidation("transfer_state", TransferProcessState); err != nil {
		return err
	}
	if err := v.RegisterValidation("contract_state", ContractNegotiationState); err != nil {
		return err
	}
	return odrl.RegisterValidators(v)
}

// GetDSPContext returns the DSP context.
// TODO: Replace all the hardcoded ones with this function.
func GetDSPContext() jsonld.Context {
	return jsonld.NewRootContext([]jsonld.ContextEntry{{ID: constants.DSPContext}})
}
