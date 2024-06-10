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

package logging

import (
	"log/slog"
	"net/http"
)

// NewMiddleware creates a middleware function that injects the logger into the context of the
// request, including some useful tags.
func NewMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger = logger.With(
				"method", r.Method,
				"host", r.Host,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"ip", r.RemoteAddr,
			)
			r = r.WithContext(Inject(ctx, logger))
			next.ServeHTTP(w, r)
		})
	}
}
