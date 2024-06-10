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
	"errors"
	"testing"
)

//nolint:funlen,lll
func TestSendContractRequest(t *testing.T) {
	t.Parallel()

	tests := []stateMachineTestCase{
		{
			name:        "Error: DSPStateStorageService.FindState() returns an error",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					findStateError: errors.New("no state"),
				},
			},
			wantErr:     true,
			expectedErr: "no state",
		},
		{
			name:        "Error: Initial state is not UndefinedState",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Requested,
				},
			},
			wantErr:     true,
			expectedErr: "status 42: err Contract negotiation state invalid. Got REQUESTED, expected UndefinedState",
		},
		{
			name:        "Error: consumerService.SendContractRequest() returns an error",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: UndefinedState,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractRequestError: errors.New("Connection broke"),
				},
			},
			wantErr:     true,
			expectedErr: "Connection broke",
		},
		{
			name:        "Error: expecting ContractNegotiationMessage, but getting other",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: UndefinedState,
				},
				consumerService: &fakeConsumerContractTasksService{},
			},
			wantErr:     true,
			expectedErr: "status 42: err Unexpected message type received after sending contract request",
		},
		{
			name:        "Error: Failing to store REQUESTED state on ContractNegotiationMessage",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: UndefinedState,
					storeStateError:  errors.New("no storing REQUESTED for you"),
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractRequestMessageType: ContractNegotiationMessage,
				},
			},
			wantErr:              false,
			expectedArgErrStatus: 42,
			expectedArgErrMsg:    "Failed to store REQUESTED state",
			wantState:            sendContractErrorMessage,
		},
		{
			name:        "Success: Exiting statemachine due to asynchronous communication",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: UndefinedState,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractRequestMessageType: ContractNegotiationMessage,
				},
			},
			wantErr:   false,
			wantState: nil,
		},
		{
			name:        "Error: Failing to store OFFERED state on ContractNegotiationMessage",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: UndefinedState,
					storeStateError:  errors.New("no storing OFFERED for you"),
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractRequestMessageType: ContractOfferMessage,
				},
			},
			wantErr:              false,
			expectedArgErrStatus: 42,
			expectedArgErrMsg:    "Failed to store OFFERED state",
			wantState:            sendContractErrorMessage,
		},
		{
			name:        "Success: Next state contract accepted on synchronous communication",
			stateMethod: sendContractRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{},
				consumerService: &fakeConsumerContractTasksService{
					sendContractRequestMessageType: ContractOfferMessage,
				},
			},
			wantErr:   false,
			wantState: sendContractAcceptedRequest,
		},
	}

	runTests(t, tests)
}
