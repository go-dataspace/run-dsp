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
	"reflect"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

func methodName(method any) string {
	if method == nil {
		return "<nil>"
	}

	return strings.TrimSuffix(runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name(), "-fm")
}

type fakeConsumerContractTasksService struct {
	sendContractRequestMessageType ContractNegotiationMessageType
	negotiationState               ContractNegotiationState
	sendContractRequestError       error

	consumerContractTasksService
}

func (f *fakeConsumerContractTasksService) SendContractRequest(ctx context.Context, args ContractArgs, odrlOffer string) (ContractNegotiationMessageType, error) {
	return f.sendContractRequestMessageType, f.sendContractRequestError
}

type fakeDSPStateStorageService struct {
	negotiationState ContractNegotiationState
	generatedUUID    uuid.UUID
	findStateError   error
	storeStateError  error
	uuidError        error

	DSPStateStorageService
}

func (f *fakeDSPStateStorageService) FindState(ctx context.Context, id uuid.UUID) (ContractNegotiationState, error) {
	return f.negotiationState, f.findStateError
}

func (f *fakeDSPStateStorageService) StoreState(ctx context.Context, id uuid.UUID, negotationState ContractNegotiationState) error {
	return f.storeStateError
}

func (f *fakeDSPStateStorageService) GenerateProcessId(ctx context.Context) (processId uuid.UUID, error error) {
	return f.generatedUUID, f.uuidError
}
