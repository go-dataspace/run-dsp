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

// //nolint:funlen,lll
// func TestSendTerminateContractNegotiation(t *testing.T) {
// 	t.Parallel()

// 	tests := []stateMachineTestCase{
// 		{
// 			name:        "Error: DSPStateStorageService.FindState() returns an error",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					findStateError: errors.New("no state"),
// 				},
// 			},
// 			wantErr:     true,
// 			expectedErr: "no state",
// 		},
// 		{
// 			name:        "Error: Initial state is Finalized",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: UndefinedState,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{},
// 			},
// 			wantErr:     true,
// 			expectedErr: "status 42: err Contract negotiation state invalid. Got UndefinedState, expected [REQUESTED OFFERED ACCEPTED AGREED VERIFIED]",
// 		},
// 		{
// 			name:        "Error: Initial state is Finalized",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Finalized,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{},
// 			},
// 			wantErr:     true,
// 			expectedErr: "status 42: err Contract negotiation state invalid. Got FINALIZED, expected [REQUESTED OFFERED ACCEPTED AGREED VERIFIED]",
// 		},
// 		{
// 			name:        "Error: consumerService.SendContractAgreementVerification() returns an error",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Agreed,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					sendContractTerminationRequestError: errors.New("Connection broke"),
// 				},
// 			},
// 			wantErr:     true,
// 			expectedErr: "Connection broke",
// 		},
// 		{
// 			name:        "Error: expecting ContractNegotiationMessage, but getting other",
// 			stateMethod: sendContractVerifiedRequest,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Agreed,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{},
// 			},
// 			wantErr:     true,
// 			expectedErr: "status 42: err Unexpected message type. Expected [ContractNegotiationMessage ContractNegotiationEventMessage], got UndefinedMessage.",
// 		},
// 		{
// 			name:        "Error: Failing to store VERIFIED state on ContractNegotiationMessage",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Agreed,
// 					storeStateError:  errors.New("no storing TERMINATED for you"),
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					sendContractTerminationRequestMessageType: ContractNegotiationMessage,
// 				},
// 			},
// 			wantErr:              false,
// 			expectedArgErrStatus: 42,
// 			expectedArgErrMsg:    "Failed to store TERMINATED state",
// 			wantState:            sendContractErrorMessage,
// 		},
// 		{
// 			name:        "Success: Exiting statemachine due to asynchronous communication",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Agreed,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					sendContractTerminationRequestMessageType: ContractNegotiationMessage,
// 				},
// 			},
// 			wantErr:   false,
// 			wantState: nil,
// 		},
// 		{
// 			name:        "Error: Failing to store TERMINATED state on ContractNegotiationEventMessage",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Agreed,
// 					storeStateError:  errors.New("no storing TERMINATED for you"),
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					sendContractTerminationRequestMessageType: ContractNegotiationMessage,
// 				},
// 			},
// 			wantErr:              false,
// 			expectedArgErrStatus: 42,
// 			expectedArgErrMsg:    "Failed to store TERMINATED state",
// 			wantState:            sendContractErrorMessage,
// 		},
// 		{
// 			name:        "Success: Consumer service: Next state contract accepted on synchronous communication",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Agreed,
// 				},
// 				consumerService: &fakeConsumerContractTasksService{
// 					sendContractTerminationRequestMessageType: ContractNegotiationMessage,
// 				},
// 			},
// 			wantErr:   false,
// 			wantState: nil,
// 		},
// 		{
// 			name:        "Success: Provider service: Next state contract accepted on synchronous communication",
// 			stateMethod: sendTerminateContractNegotiation,
// 			args: ContractArgs{
// 				BaseArgs: BaseArgs{ParticipantRole: Provider},
// 				StateStorage: &fakeDSPStateStorageService{
// 					negotiationState: Agreed,
// 				},
// 				providerService: &fakeProviderContractTasksService{
// 					sendContractTerminationRequestMessageType: ContractNegotiationMessage,
// 				},
// 			},
// 			wantErr:   false,
// 			wantState: nil,
// 		},
// 	}

// 	runTests(t, tests, "TestSendTerminateContractNegotiation")
// }
