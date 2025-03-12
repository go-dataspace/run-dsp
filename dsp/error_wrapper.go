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
	"errors"
	"net/http"

	"codeberg.org/go-dataspace/run-dsp/dsp/shared"
	"codeberg.org/go-dataspace/run-dsp/logging"
)

// HTTPReturnError is an interface for a dataspace protocol error, containing all the information
// needed to return a sane dataspace error over HTTP.
type HTTPReturnError interface {
	error
	StatusCode() int
	ErrorType() string
	DSPCode() string
	Reason() []shared.Multilanguage
	Description() []shared.Multilanguage
	ProviderPID() string
	ConsumerPID() string
}

// WrapHandlerWithError wraps a http handler that returns an error into a more generic http.Handler.
// It will handle the error like it's supposed to be an http error. If the function returns a normal
// error, it will return a 500 with a generic error message. If the error conforms to a HTTPError,
// it will use the information to format a proper HTTP dataspace error.
func WrapHandlerWithError(h func(w http.ResponseWriter, r *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err != nil {
			logger := logging.Extract(r.Context())
			logger.Error("HTTP handler returned error", "err", err.Error())

			var httpError HTTPReturnError
			if errors.As(err, &httpError) {
				handleHTTPError(w, r, httpError)
				return
			}

			// If a normal error we just return a generic 500 with a generic error.
			if err := shared.EncodeValid(w, r, http.StatusInternalServerError, shared.DSPError{
				Context: shared.GetDSPContext(),
				Type:    "dspace:UnknownError",
				Code:    "INTERNAL",
				Reason: []shared.Multilanguage{
					{
						Value:    "Internal Server Errror",
						Language: "en",
					},
				},
			}); err != nil {
				logger.Error("Error while encoding generic error", "err", err)
			}
		}
	})
}

func handleHTTPError(w http.ResponseWriter, r *http.Request, err HTTPReturnError) {
	dErr := shared.DSPError{
		Context:     shared.GetDSPContext(),
		Type:        err.ErrorType(),
		ProviderPID: err.ProviderPID(),
		ConsumerPID: err.ConsumerPID(),
		Code:        err.DSPCode(),
		Reason:      err.Reason(),
		Description: err.Description(),
	}
	if err := shared.EncodeValid(w, r, err.StatusCode(), dErr); err != nil {
		logging.Extract(r.Context()).Error("Error while encoding HTTP Error", "err", err)
	}
}
