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

// Package authforwarder is a combination of http middleware and grpc injectors that passes the
// content of the "Authorization" header from the incoming http request to any outgoing grpc calls.
package authforwarder

import (
	"context"
	"fmt"
	"net/http"

	"go-dataspace.eu/ctxslog"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

type contextKeyType string

const (
	contextKey contextKeyType = "authheader"
)

// HTTPMiddleware injects the authorization header into the context.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		authContents := req.Header.Get("Authorization")
		req = req.WithContext(context.WithValue(req.Context(), contextKey, authContents))
		next.ServeHTTP(w, req)
	})
}

// AuthRoundTripper is a http client "middleware" that extracts the auth middleware out of the
// context and injects it into the request.
type AuthRoundTripper struct {
	Proxied      http.RoundTripper
	AuthNService dsrpc.AuthNServiceClient
}

// Roundtrip does the actual injection.
func (art AuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctxslog.Warn(req.Context(), "Doing request", "method", req.Method, "url", req.URL.String())
	if art.AuthNService != nil {
		authVal, err := art.AuthNService.Sign(req.Context(), &dsrpc.SignRequest{
			RequestInfo: &dsrpc.RequestInfo{
				Method: req.Method,
				Url:    req.URL.String(),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("could not sign outgoing request: %w", err)
		}
		req.Header.Add("Authorization", authVal.GetAuthenticationHeaderValue())
	}
	return art.Proxied.RoundTrip(req)
}
