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
	"errors"
	"testing"

	"github.com/go-dataspace/run-dsp/logging"
)

//nolint:funlen,lll
func TestSendContractRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        ContractArgs
		wantErr     bool
		expectedErr string
		wantState   DSPState[ContractArgs]
	}{
		{
			name: "Error: DSPStateStorageService.FindState() returns an error",
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: 0,
					findStateError:   errors.New("no state"),
				},
				// consumerService: &fakeConsumerContractTasksService{
				// 	sendContractRequestMessageType: 0,
				// 	negotiationState:               0,
				// 	sendContractRequestError:       nil,
				// 	consumerContractTasksService:   nil,
				// },
			},
			wantErr:     true,
			expectedErr: "no state",
			wantState:   nil,
		},
		{
			name: "Error: Initial state is not UndefinedState",
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: Requested,
				},
			},
			wantErr:     true,
			expectedErr: "Initial contract negotiation state invalid, should be UndefinedState",
			wantState:   nil,
		},
		{
			name: "Error: consumerService.SendContractRequest() returns an error",
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: UndefinedState,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractRequestMessageType: 0,
					negotiationState:               0,
					sendContractRequestError:       errors.New("Connection broke"),
					consumerContractTasksService:   nil,
				},
			},
			wantErr:     true,
			expectedErr: "Connection broke",
			wantState:   nil,
		},
		{
			name: "Error: expecting ContractNegotiationMessage, but getting other",
			args: ContractArgs{
				StateStorage: &fakeDSPStateStorageService{
					negotiationState: UndefinedState,
				},
				consumerService: &fakeConsumerContractTasksService{
					sendContractRequestMessageType: 0,
					negotiationState:               0,
					sendContractRequestError:       nil,
					consumerContractTasksService:   nil,
				},
			},
			wantErr:     true,
			expectedErr: "Connection broke",
			wantState:   nil,
		},
	}

	for _, test := range tests {
		ctx := logging.Inject(context.Background(), logging.NewJSON("debug", true))
		_, nextState, err := sendContractRequest(ctx, test.args)
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

		gotState := methodName(nextState)
		wantState := methodName(test.wantState)
		if gotState != wantState {
			t.Errorf("TestsendContractRequest(%s): got next state %s, want %s", test.name, gotState, wantState)
		}
	}
}
