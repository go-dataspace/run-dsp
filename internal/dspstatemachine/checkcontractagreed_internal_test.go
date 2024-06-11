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
func TestCheckContractAgreedRequest(t *testing.T) {
	t.Parallel()

	tests := []stateMachineTestCase{
		{
			name:        "Error: DSPStateStorageService.FindState() returns an error",
			stateMethod: checkContractAgreedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					findStateError: errors.New("no state"),
				},
			},
			wantErr:     true,
			expectedErr: "no state",
		},
		{
			name:        "Error: Initial state is not Accepted",
			stateMethod: checkContractAgreedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Requested,
				},
			},
			wantErr:     true,
			expectedErr: "status 42: err Contract negotiation state invalid. Got REQUESTED, expected [ACCEPTED]",
		},
		{
			name:        "Error: consumerService.CheckContractOffer() returns an error",
			stateMethod: checkContractAgreedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Accepted,
				},
				consumerService: &fakeConsumerContractTasksService{
					checkContractAgreedRequestError: errors.New("Connection broke"),
				},
			},
			wantErr:     true,
			expectedErr: "Connection broke",
		},
		{
			name:        "Error: DSPStateStorageService.StoreState() returns an error",
			stateMethod: checkContractAgreedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Accepted,
					storeStateError:  errors.New("no state"),
				},
				consumerService: &fakeConsumerContractTasksService{},
			},
			wantErr:              false,
			expectedErr:          "",
			expectedArgErrStatus: 42,
			expectedArgErrMsg:    "Failed to store AGREED state",
			wantState:            sendContractErrorMessage,
		},
		{
			name:        "Error: Asynchronous communication not able to send ACK message",
			stateMethod: checkContractAgreedRequest,
			args: ContractArgs{
				BaseArgs: BaseArgs{AsynchronousCommunication: true},
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Accepted,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractNegotiationRequestError: errors.New("broken"),
				},
			},
			wantErr:     true,
			expectedErr: "broken",
		},
		{
			name:        "Success: Next state contract verified",
			stateMethod: checkContractAgreedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Accepted,
				},
				consumerService: &fakeConsumerContractTasksService{
					contractOfferAgreed: true,
				},
			},
			wantErr:   false,
			wantState: sendContractVerifiedRequest,
		},
		{
			name:        "Terminated: Next state contract terminated",
			stateMethod: checkContractAgreedRequest,
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Accepted,
				},
				consumerService: &fakeConsumerContractTasksService{
					contractOfferAgreed: false,
				},
			},
			wantErr:   false,
			wantState: sendTerminateContractNegotiation,
		},
	}

	runTests(t, tests, "TestCheckContractAgreedRequest")
}
