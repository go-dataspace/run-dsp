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

//nolint:funlen
func TestSendContractVerifiedRequest(t *testing.T) {
	t.Parallel()

	tests := []stateMachineTestCase{
		{
			name:        "Error: DSPStateStorageService.FindState() returns an error",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					findStateError: errors.New("no state"),
				},
			},
			wantErr:     true,
			expectedErr: "no state",
		},
		{
			name:        "Error: Initial state is not Agreed",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Requested,
				},
			},
			wantErr:     true,
			expectedErr: "status 42: err Contract negotiation state invalid. Got REQUESTED, expected [AGREED]",
		},
		{
			name:        "Error: consumerService.SendContractAgreementVerification() returns an error",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Agreed,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractVerifiedRequestError: errors.New("Connection broke"),
				},
			},
			wantErr:     true,
			expectedErr: "Connection broke",
		},
		{
			name:        "Error: expecting ContractNegotiationMessage, but getting other",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Agreed,
				},
				consumerService: &fakeConsumerContractTasksService{},
			},
			wantErr:     true,
			expectedErr: "status 42: err Unexpected message type. Expected [ContractNegotiationMessage ContractNegotiationEventMessage], got UndefinedMessage.",
		},
		{
			name:        "Error: Failing to store VERIFIED state on ContractNegotiationMessage",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Agreed,
					storeStateError:  errors.New("no storing VERIFIED for you"),
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractVerifiedRequestMessageType: ContractNegotiationMessage,
				},
			},
			wantErr:              false,
			expectedArgErrStatus: 42,
			expectedArgErrMsg:    "Failed to store VERIFIED state",
			wantState:            sendContractErrorMessage,
		},
		{
			name:        "Success: Exiting statemachine due to asynchronous communication",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Agreed,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractVerifiedRequestMessageType: ContractNegotiationMessage,
				},
			},
			wantErr:   false,
			wantState: nil,
		},
		{
			name:        "Error: Failing to store FINALIZED state on ContractNegotiationEventMessage",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Agreed,
					storeStateError:  errors.New("no storing FINALIZED for you"),
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractVerifiedRequestMessageType: ContractNegotiationEventMessage,
				},
			},
			wantErr:              false,
			expectedArgErrStatus: 42,
			expectedArgErrMsg:    "Failed to store FINALIZED state",
			wantState:            sendContractErrorMessage,
		},
		{
			name:        "Success: Next state contract accepted on synchronous communication",
			stateMethod: sendContractVerifiedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Agreed,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractVerifiedRequestMessageType: ContractNegotiationEventMessage,
				},
			},
			wantErr:   false,
			wantState: nil,
		},
	}

	runTests(t, tests, "TestSendContractVerifiedRequest")
}
