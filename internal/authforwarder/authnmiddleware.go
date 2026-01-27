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

	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

const (
	RequesterInfoContextKey contextKeyType = "requesterinfo"
)

func NewAuthNMiddleware(authNService dsrpc.AuthNServiceClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			authContents := ExtractAuthorization(req.Context())
			requesterInfo := &dsrpc.RequesterInfo{
				AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_UNAUTHENTICATED,
			}

			if authNService != nil && authContents != "" {
				verifyResponse, err := authNService.Verify(req.Context(), &dsrpc.VerifyRequest{
					AuthenticationHeaderValue: authContents,
					RequestInfo: &dsrpc.RequestInfo{
						Method: req.Method,
						Url:    req.URL.String(),
					},
				})
				if err != nil {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				requesterInfo = verifyResponse.RequesterInfo
			}

			req = req.WithContext(context.WithValue(req.Context(), RequesterInfoContextKey, requesterInfo))
			next.ServeHTTP(w, req)
		})
	}
}

func ExtractRequesterInfo(ctx context.Context) *dsrpc.RequesterInfo {
	ctxVal := ctx.Value(RequesterInfoContextKey)
	if ctxVal == nil {
		panic("requesterInfo not set in context")
	}
	val, ok := ctxVal.(*dsrpc.RequesterInfo)
	if !ok {
		panic("requesterInfo from context not of right type")
	}
	return val
}

func SetRequesterInfo(ctx context.Context, requesterInfo *dsrpc.RequesterInfo) context.Context {
	return context.WithValue(ctx, RequesterInfoContextKey, requesterInfo)
}

func CheckRequesterInfo(ctx context.Context, stored *dsrpc.RequesterInfo) error {
	if stored.AuthenticationStatus == dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_UNAUTHENTICATED ||
		stored.AuthenticationStatus == dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN {
		return nil
	}

	requesterInfo := ExtractRequesterInfo(ctx)
	if requesterInfo.AuthenticationStatus == dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN ||
		requesterInfo.AuthenticationStatus == dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_CONTINUATION {
		return nil
	}

	if requesterInfo.Identifier != stored.Identifier {
		return fmt.Errorf("identifier mismatch in RequesterInfo")
	}

	return nil
}
