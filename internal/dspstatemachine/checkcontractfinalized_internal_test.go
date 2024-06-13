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

// //nolint:funlen
// func TestCheckContractFinalizedRequest(t *testing.T) {
// 	t.Parallel()

// 	tests := []stateMachineTestCase{
// 		{
// 			name:        "Error: DSPStateStorageService.FindState() returns an error",
// 			stateMethod: checkContractFinalizedRequest,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					findStateError: errors.New("no state"),
// 				},
// 			},
// 			wantErr:     true,
// 			expectedErr: "no state",
// 		},
// 		{
// 			name:        "Error: Initial state is not Accepted",
// 			stateMethod: checkContractFinalizedRequest,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Requested,
// 				},
// 			},
// 			wantErr:     true,
// 			expectedErr: "status 42: err Contract negotiation state invalid. Got REQUESTED, expected [VERIFIED]",
// 		},
// 		{
// 			name:        "Error: consumerService.CheckContractFinalized() returns an error",
// 			stateMethod: checkContractFinalizedRequest,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Verified,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					checkContractFinalizedRequestError: errors.New("Connection broke"),
// 				},
// 			},
// 			wantErr:     true,
// 			expectedErr: "Connection broke",
// 		},
// 		{
// 			name:        "Error: DSPStateStorageService.StoreState() returns an error",
// 			stateMethod: checkContractFinalizedRequest,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Verified,
// 					storeStateError:  errors.New("no state"),
// 				},
// 				consumerService: &fakeConsumerContractTasksService{},
// 			},
// 			wantErr:              false,
// 			expectedErr:          "",
// 			expectedArgErrStatus: 42,
// 			expectedArgErrMsg:    "Failed to store FINALIZED state",
// 			wantState:            sendContractErrorMessage,
// 		},
// 		{
// 			name:        "Error: Asynchronous communication false, but trying to and not able to send ACK message",
// 			stateMethod: checkContractFinalizedRequest,
// 			args: ContractArgs{
// 				BaseArgs: BaseArgs{AsynchronousCommunication: false},
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Verified,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					sendContractNegotiationRequestError: errors.New("broken"),
// 				},
// 			},
// 			wantErr:     true,
// 			expectedErr: "broken",
// 		},
// 		{
// 			name:        "Success: Next state contract verified",
// 			stateMethod: checkContractFinalizedRequest,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Verified,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					sendContractNegotiationRequestMessageType: ContractNegotiationMessage,
// 				},
// 			},
// 			wantErr:   false,
// 			wantState: nil,
// 		},
// 	}

// 	runTests(t, tests, "TestCheckContractFinalizedRequest")
// }
