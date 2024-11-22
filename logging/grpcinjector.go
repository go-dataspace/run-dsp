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
	"context"
	"log/slog"

	"google.golang.org/grpc"
)

// UnaryServerInterceptor injects the logger in the context.
func UnaryServerInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (any, error) {
		reqLogger := logger.With(
			"type", "gRPC unary call",
			"method", info.FullMethod,
		)
		ctx = Inject(ctx, reqLogger)
		return handler(ctx, req)
	}
}

type serverStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStream) Context() context.Context {
	return s.ctx
}

func StreamServerInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		reqLogger := logger.With(
			"type", "gRPC streaming call",
			"method", info.FullMethod,
			"client_stream", info.IsClientStream,
			"server_stream", info.IsServerStream,
		)
		ctx = Inject(ctx, reqLogger)
		return handler(srv, &serverStream{ss, ctx})
	}
}
