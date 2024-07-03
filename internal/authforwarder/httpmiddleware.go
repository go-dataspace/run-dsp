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
	"net/http"
)

type contextKeyType string

const (
	contextKey contextKeyType = "authheader"
)

func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		authContents := req.Header.Get("Authorization")
		req = req.WithContext(context.WithValue(req.Context(), contextKey, authContents))
		next.ServeHTTP(w, req)
	})
}
