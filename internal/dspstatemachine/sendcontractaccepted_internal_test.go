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
func TestSendAcceptedRequest(t *testing.T) {
	t.Parallel()

	tests := []stateMachineTestCase{
		{
			name:        "Error: DSPStateStorageService.FindState() returns an error",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					findStateError: errors.New("no state"),
				},
			},
			wantErr:     true,
			expectedErr: "no state",
		},
		{
			name:        "Error: Initial state is not Offered",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Requested,
				},
			},
			wantErr:     true,
			expectedErr: "status 42: err Contract negotiation state invalid. Got REQUESTED, expected [OFFERED]",
		},
		{
			name:        "Error: consumerService.SendContractAccepted() returns an error",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Offered,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractAcceptedRequestError: errors.New("Connection broke"),
				},
			},
			wantErr:     true,
			expectedErr: "Connection broke",
		},
		{
			name:        "Error: expecting ContractNegotiationMessage, but getting other",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Offered,
				},
				consumerService: &fakeConsumerContractTasksService{},
			},
			wantErr: true,
			//nolint:lll
			expectedErr: "status 42: err Unexpected message type. Expected [ContractNegotiationMessage ContractAgreementMessage], got UndefinedMessage.",
		},
		{
			name:        "Error: Failing to store OFFERED state on ContractNegotiationMessage",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Offered,
					storeStateError:  errors.New("no storing ACCEPTED for you"),
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractAcceptedRequestMessageType: ContractNegotiationMessage,
				},
			},
			wantErr:              false,
			expectedArgErrStatus: 42,
			expectedArgErrMsg:    "Failed to store ACCEPTED state",
			wantState:            sendContractErrorMessage,
		},
		{
			name:        "Success: Exiting statemachine due to asynchronous communication",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Offered,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractAcceptedRequestMessageType: ContractNegotiationMessage,
				},
			},
			wantErr:   false,
			wantState: nil,
		},
		{
			name:        "Error: Failing to store AGREED state on ContractAgreementMessage",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Offered,
					storeStateError:  errors.New("no storing OFFERED for you"),
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractAcceptedRequestMessageType: ContractAgreementMessage,
				},
			},
			wantErr:              false,
			expectedArgErrStatus: 42,
			expectedArgErrMsg:    "Failed to store AGREED state",
			wantState:            sendContractErrorMessage,
		},
		{
			name:        "Success: Next state contract accepted on synchronous communication",
			stateMethod: sendContractAcceptedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Offered,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractAcceptedRequestMessageType: ContractAgreementMessage,
				},
			},
			wantErr:   false,
			wantState: sendContractVerifiedRequest,
		},
	}

	runTests(t, tests, "TestSendAcceptedRequest")
}
