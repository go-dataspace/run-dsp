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

package authforwarder

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")

// UnaryClientInterceptor extracts the auth header value from the context and appends it to the
// outgoing metadata.
func UnaryClientInterceptor(
	ctx context.Context,
	method string, req interface{}, reply interface{},
	cc *grpc.ClientConn, invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	val := ExtractAuthorization(ctx)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", val)
	return invoker(ctx, method, req, reply, cc, opts...)
}

// StreamClientInterceptor does the same for streaming requests. Note that we don't try
// to intercept the streaming messages.
func StreamClientInterceptor(
	ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
	method string, streamer grpc.Streamer, opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	val := ExtractAuthorization(ctx)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", val)
	return streamer(ctx, desc, cc, method, opts...)
}

func UnaryInterceptor(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}
	authContents := md["authorization"]
	ctx = context.WithValue(ctx, contextKey, authContents)
	return handler(ctx, req)
}

type serverStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStream) Context() context.Context {
	return s.ctx
}

// StreamInterceptor will process the authorization header, convert it to a prefix, and then
// inject it into the context, but for streams.
func StreamInterceptor(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	ctx := ss.Context()
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errMissingMetadata
	}
	authContents := md["authorization"]
	ctx = context.WithValue(ctx, contextKey, authContents)
	return handler(srv, &serverStream{ss, ctx})
}

// ExtractAuthorization shouldn't be public, but it temporarily is to keep things working.
func ExtractAuthorization(ctx context.Context) string {
	ctxVal := ctx.Value(contextKey)
	if ctxVal == nil {
		return ""
	}
	val, ok := ctxVal.(string)
	if !ok {
		panic("Auth header not of right type")
	}
	return val
}
