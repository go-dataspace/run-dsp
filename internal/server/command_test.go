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

package server_test

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go-dataspace.eu/run-dsp/internal/server"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type fakeContractService struct {
	dsrpc.UnimplementedContractServiceServer
	configured atomic.Int32
	lastAddr   atomic.Pointer[string]
}

func (s *fakeContractService) Configure(
	_ context.Context,
	req *dsrpc.ContractServiceConfigureRequest,
) (*dsrpc.ContractServiceConfigureResponse, error) {
	s.configured.Add(1)
	addr := req.GetConnectorAddress()
	s.lastAddr.Store(&addr)
	return &dsrpc.ContractServiceConfigureResponse{}, nil
}

func startServer(
	t *testing.T,
	svc dsrpc.ContractServiceServer,
) (*grpc.Server, *bufconn.Listener) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	dsrpc.RegisterContractServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	return srv, lis
}

func TestWatchContractServiceReconfiguresOnReconnect(t *testing.T) {
	const controlAddr = "run-dsp:8081"
	svc := &fakeContractService{}
	srv, lis := startServer(t, svc)
	var current atomic.Pointer[bufconn.Listener]
	current.Store(lis)
	bc := backoff.DefaultConfig
	bc.BaseDelay = 10 * time.Millisecond
	bc.MaxDelay = 100 * time.Millisecond
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return current.Load().DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: bc, MinConnectTimeout: time.Second}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	wg := &sync.WaitGroup{}
	server.WatchContractService(
		ctx, wg, conn,
		dsrpc.NewContractServiceClient(conn),
		controlAddr)
	require.Eventually(
		t,
		func() bool { return svc.configured.Load() == 1 },
		5*time.Second,
		10*time.Millisecond,
		"first Configure after the connection became ready")
	require.NotNil(t, svc.lastAddr.Load())
	require.Equal(t, controlAddr, *svc.lastAddr.Load())
	srv2, lis2 := startServer(t, svc)
	t.Cleanup(srv2.Stop)
	current.Store(lis2)
	srv.Stop()
	require.Eventually(
		t,
		func() bool { return svc.configured.Load() == 2 },
		5*time.Second,
		10*time.Millisecond,
		"configure after the server was replaced")
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not stop on context cancel")
	}
}
