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
	"testing"

	"github.com/go-dataspace/run-dsp/logging"
	"github.com/google/uuid"
)

// TODO: Remove this comment.
func methodName(method any) string {
	if method == nil {
		return "<nil>"
	}

	return strings.TrimSuffix(runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name(), "-fm")
}

type fakeConsumerContractTasksService struct {
	sendContractRequestMessageType         ContractNegotiationMessageType
	sendContractAcceptedRequestMessageType ContractNegotiationMessageType
	// negotiationState                       ContractNegotiationState
	sendContractRequestError         error
	sendContractAcceptedRequestError error

	consumerContractTasksService
}

func (f *fakeConsumerContractTasksService) SendContractRequest(
	ctx context.Context, args ContractArgs) (
	ContractNegotiationMessageType, error,
) {
	return f.sendContractRequestMessageType, f.sendContractRequestError
}

func (f *fakeConsumerContractTasksService) SendContractAccepted(
	ctx context.Context, args ContractArgs) (
	ContractNegotiationMessageType, error,
) {
	return f.sendContractAcceptedRequestMessageType, f.sendContractAcceptedRequestError
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

func (f *fakeDSPStateStorageService) StoreState(ctx context.Context, id uuid.UUID, cns ContractNegotiationState) error {
	return f.storeStateError
}

func (f *fakeDSPStateStorageService) GenerateProcessId(ctx context.Context) (uuid.UUID, error) {
	return f.generatedUUID, f.uuidError
}

type stateMachineTestCase struct {
	name                 string
	stateMethod          DSPState[ContractArgs]
	args                 ContractArgs
	wantErr              bool
	expectedErr          string
	expectedArgErrMsg    string
	expectedArgErrStatus int
	wantState            DSPState[ContractArgs]
}

//nolint:cyclop
func runTests(t *testing.T, tests []stateMachineTestCase) {
	for _, test := range tests {
		ctx := logging.Inject(context.Background(), logging.NewJSON("debug", true))
		args, nextState, err := test.stateMethod(ctx, test.args)
		switch {
		case err == nil && test.wantErr:
			t.Errorf("TestsendContractRequest(%s): got err == nil, want err != nil", test.name)
			continue
		case err != nil && !test.wantErr:
			t.Errorf("TestsendContractRequest(%s): got err == '%s', want err == nil", test.name, err)
			continue
		case err != nil:
			if test.expectedErr != "" && test.expectedErr != err.Error() {
				t.Errorf("TestsendContractRequest(%s): got err == '%s', want err == '%s'", test.name, err, test.expectedErr)
			}
			continue
		}

		if test.expectedArgErrMsg != args.ErrorMessage {
			t.Errorf(
				"TestsendContractRequest(%s): got args.ErrorMessage == '%s', want args.ErrorMessage == '%s'",
				test.name, args.ErrorMessage, test.expectedArgErrMsg)
		}
		if test.expectedArgErrStatus != args.StatusCode {
			t.Errorf(
				"TestsendContractRequest(%s): got args.StatusCode == '%d', want args.StatusCode == '%d'",
				test.name, args.StatusCode, test.expectedArgErrStatus)
		}
		gotState := methodName(nextState)
		wantState := methodName(test.wantState)
		if gotState != wantState {
			t.Errorf("TestsendContractRequest(%s): got next state %s, want %s", test.name, gotState, wantState)
		}
	}
}
