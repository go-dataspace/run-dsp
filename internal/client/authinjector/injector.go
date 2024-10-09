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

// Package authinjector contains a helper injector that injects an argument in the metadata.
package authinjector

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// InjectUnaryAuthInterceptor returns a unary interceptor that injects `val` into the `authorization` metadata.
func InjectUnaryAuthInterceptor(val string) func(
	ctx context.Context,
	method string, req interface{}, reply interface{},
	cc *grpc.ClientConn, invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	return func(
		ctx context.Context,
		method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", val)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// InjectStreamAuthInterceptor returns a stream interceptor that injects `val` into the `authorization` metadata.
func InjectStreamAuthInterceptor(val string) func(
	ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
	method string, streamer grpc.Streamer, opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	return func(
		ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", val)
		return streamer(ctx, desc, cc, method, opts...)
	}
}
