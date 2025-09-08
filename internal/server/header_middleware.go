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

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
)

// jsonHeaderMiddleware adds the json header to the response and checks if the content-type is json
// for POST requests.
func jsonHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		contentTypes := []string{"application/json", "application/json; charset=utf-8"}
		if r.Header.Get("Content-Length") != "" &&
			!slices.Contains(contentTypes, r.Header.Get("Content-Type")) {
			w.WriteHeader(http.StatusBadRequest)
			resp, err := json.Marshal(struct {
				Error string
			}{
				Error: fmt.Sprintf("Unsupported content-type: %s", r.Header.Get("Content-Type")),
			})
			if err != nil {
				panic(err.Error())
			}
			_, _ = fmt.Fprint(w, string(resp))
			return
		}
		next.ServeHTTP(w, r)
	})
}
