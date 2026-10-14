// Copyright 2026 go-dataspace
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

// WatchContractService exposes watchContractService to the external test package.
package server

import (
	"context"
	"sync"

	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"google.golang.org/grpc"
)

func WatchContractService(
	ctx context.Context,
	wg *sync.WaitGroup,
	conn *grpc.ClientConn,
	contractService dsrpc.ContractServiceClient,
	controlAddr string,
) {
	c := &command{ControlExternalAddr: controlAddr}
	c.watchContractService(ctx, wg, conn, contractService)
}
