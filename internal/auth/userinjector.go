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

// Package auth contains auth or, in this case, fake auth middleware.
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type contextKeyType string

const (
	contextKey contextKeyType = "userinfo"
	dateFormat string         = "2006-01-02"
)

// UserInfo struct that will be injected into the context.
type UserInfo struct {
	FirstName string
	Lastname  string
	BirthDate time.Time
}

func (ui UserInfo) String() string {
	return fmt.Sprintf("%s;%s;%s", ui.FirstName, ui.Lastname, ui.BirthDate.Format(dateFormat))
}

// NonsenseUserInjector is a temporary auth injector.
// DO NOT USE IN PRODUCTION
// The format for the auth header is FirstName;Lastname;YYYY-MM-DD.
func NonsenseUserInjector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		parts := strings.Split(req.Header.Get("Authorization"), ";")
		if len(parts) != 3 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		bday, err := time.Parse(dateFormat, parts[2])
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ui := UserInfo{
			FirstName: parts[0],
			Lastname:  parts[1],
			BirthDate: bday,
		}
		req = req.WithContext(context.WithValue(req.Context(), contextKey, &ui))
		next.ServeHTTP(w, req)
	})
}

func ExtractUserInfo(ctx context.Context) *UserInfo {
	ctxVal := ctx.Value(contextKey)
	if ctxVal == nil {
		return nil
	}

	ui, ok := ctxVal.(*UserInfo)
	if !ok {
		panic("UserInfo not of right type")
	}
	return ui
}
