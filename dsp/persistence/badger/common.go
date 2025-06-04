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

package badger

import (
	"context"
	"fmt"

	"go-dataspace.eu/run-dsp/internal/authforwarder"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

func checkRequesterInfo(ctx context.Context, stored *dsrpc.RequesterInfo) error {
	if stored.AuthenticationStatus == dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN {
		return nil
	}

	requesterInfo := authforwarder.ExtractRequesterInfo(ctx)
	if requesterInfo.Identifier != stored.Identifier {
		return fmt.Errorf("identifier mismatch in RequesterInfo")
	}

	return nil
}
