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

func methodName(method any) string {
	if method == nil {
		return "<nil>"
	}

	return strings.TrimSuffix(runtime.FuncForPC(reflect.ValueOf(method).Pointer()).Name(), "-fm")
}

type fakeConsumerContractTasksService struct {
	sendContractRequestMessageType            ContractNegotiationMessageType
	contractOfferAccepted                     bool
	contractOfferAgreed                       bool
	sendContractAcceptedRequestMessageType    ContractNegotiationMessageType
	sendContractVerifiedRequestMessageType    ContractNegotiationMessageType
	sendContractTerminationRequestMessageType ContractNegotiationMessageType
	sendContractNegotiationRequestMessageType ContractNegotiationMessageType

	// negotiationState                       ContractNegotiationState
	sendContractRequestError            error
	checkContractOfferRequestError      error
	sendContractAcceptedRequestError    error
	checkContractAgreedRequestError     error
	sendContractVerifiedRequestError    error
	checkContractFinalizedRequestError  error
	sendContractTerminationRequestError error
	sendContractNegotiationRequestError error

	consumerContractTasksService
}

func (f *fakeConsumerContractTasksService) SendContractRequest(
	ctx context.Context, args ContractArgs) (
	ContractNegotiationMessageType, error,
) {
	return f.sendContractRequestMessageType, f.sendContractRequestError
}

func (f *fakeConsumerContractTasksService) CheckContractOffer(
	ctx context.Context, args ContractArgs) (
	bool, error,
) {
	return f.contractOfferAccepted, f.checkContractOfferRequestError
}

func (f *fakeConsumerContractTasksService) SendContractAccepted(
	ctx context.Context, args ContractArgs) (
	ContractNegotiationMessageType, error,
) {
	return f.sendContractAcceptedRequestMessageType, f.sendContractAcceptedRequestError
}

func (f *fakeConsumerContractTasksService) CheckContractAgreed(
	ctx context.Context, args ContractArgs) (
	bool, error,
) {
	return f.contractOfferAgreed, f.checkContractAgreedRequestError
}

func (f *fakeConsumerContractTasksService) SendContractAgreementVerification(
	ctx context.Context, args ContractArgs) (
	ContractNegotiationMessageType, error,
) {
	return f.sendContractVerifiedRequestMessageType, f.sendContractVerifiedRequestError
}

func (f *fakeConsumerContractTasksService) CheckContractFinalized(
	ctx context.Context, args ContractArgs) (
	bool, error,
) {
	return true, f.checkContractFinalizedRequestError
}

func (f *fakeConsumerContractTasksService) SendTerminationMessage(
	ctx context.Context, args ContractArgs,
) (ContractNegotiationMessageType, error) {
	return f.sendContractTerminationRequestMessageType, f.sendContractTerminationRequestError
}

func (f *fakeConsumerContractTasksService) SendContractNegotiationMessage(
	ctx context.Context, args ContractArgs,
) (ContractNegotiationMessageType, error) {
	return f.sendContractNegotiationRequestMessageType, f.sendContractNegotiationRequestError
}

type fakeProviderContractTasksService struct {
	sendContractTerminationRequestMessageType ContractNegotiationMessageType
	sendContractTerminationRequestError       error
	providerContractTasksService
}

func (f *fakeProviderContractTasksService) SendTerminationMessage(
	ctx context.Context, args ContractArgs,
) (ContractNegotiationMessageType, error) {
	return f.sendContractTerminationRequestMessageType, f.sendContractTerminationRequestError
}

type fakeDSPStateStorageService struct {
	negotiationState ContractNegotiationState
	generatedUUID    uuid.UUID
	findStateError   error
	storeStateError  error
	uuidError        error

	DSPStateStorageService
}

func (f *fakeDSPStateStorageService) FindContractNegotiationState(ctx context.Context, id uuid.UUID) (ContractNegotiationState, error) {
	return f.negotiationState, f.findStateError
}

func (f *fakeDSPStateStorageService) StoreContractNegotiationState(ctx context.Context, id uuid.UUID, cns ContractNegotiationState) error {
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
func runTests(t *testing.T, tests []stateMachineTestCase, testName string) {
	for _, test := range tests {
		ctx := logging.Inject(context.Background(), logging.NewJSON("debug", true))
		args, nextState, err := test.stateMethod(ctx, test.args)
		switch {
		case err == nil && test.wantErr:
			t.Errorf("%s(%s): got err == nil, want err != nil", testName, test.name)
			continue
		case err != nil && !test.wantErr:
			t.Errorf("%s(%s): got err == '%s', want err == nil", testName, test.name, err)
			continue
		case err != nil:
			if test.expectedErr != "" && test.expectedErr != err.Error() {
				t.Errorf("%s(%s): got err == '%s', want err == '%s'", testName, test.name, err, test.expectedErr)
			}
			continue
		}

		if test.expectedArgErrMsg != args.ErrorMessage {
			t.Errorf(
				"%s(%s): got args.ErrorMessage == '%s', want args.ErrorMessage == '%s'",
				testName, test.name, args.ErrorMessage, test.expectedArgErrMsg)
		}
		if test.expectedArgErrStatus != args.StatusCode {
			t.Errorf(
				"%s(%s): got args.StatusCode == '%d', want args.StatusCode == '%d'",
				testName, test.name, args.StatusCode, test.expectedArgErrStatus)
		}
		gotState := methodName(nextState)
		wantState := methodName(test.wantState)
		if gotState != wantState {
			t.Errorf("%s(%s): got next state %s, want %s", testName, test.name, gotState, wantState)
		}
	}
}
