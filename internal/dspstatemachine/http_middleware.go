// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package dspstatemachine

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKeyType string

const contextKey contextKeyType = "statechannel"

type StateStorageChannelMessage struct {
	Context         context.Context
	ProcessID       uuid.UUID
	TransactionType DSPTransactionType
}

func NewMiddleware(idChan chan StateStorageChannelMessage) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req = req.WithContext(context.WithValue(req.Context(), contextKey, idChan))
			next.ServeHTTP(w, req)
		})
	}
}

func ExtractChannel(ctx context.Context) chan StateStorageChannelMessage {
	ctxVal := ctx.Value(contextKey)
	if ctxVal == nil {
		return nil
	}

	channel, ok := ctxVal.(chan StateStorageChannelMessage)
	if !ok {
		panic("This is not a valid go channel")
	}
	return channel
}
