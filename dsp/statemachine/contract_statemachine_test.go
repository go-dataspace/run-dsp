// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statemachine_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go-dataspace.eu/ctxslog"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/dsp/statemachine"
	"go-dataspace.eu/run-dsp/logging"
	mockdsrpc "go-dataspace.eu/run-dsp/mocks/go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

type MockRequester struct {
	ReceivedMethod string
	ReceivedURL    *url.URL
	ReceivedBody   []byte
	Response       []byte
}

func urlMustParse(u string) *url.URL {
	nu, err := url.Parse(u)
	if err != nil {
		panic("bad url")
	}
	return nu
}

func (mr *MockRequester) SendHTTPRequest(
	ctx context.Context, method string, url *url.URL, reqBody []byte,
) ([]byte, error) {
	mr.ReceivedMethod = method
	mr.ReceivedURL = url
	mr.ReceivedBody = reqBody
	return mr.Response, nil
}

var (
	target           = uuid.MustParse("68d3d534-06b9-4700-9890-915bc32ecb75")
	consumerPID      = uuid.MustParse("d6bc4c28-973b-4c2f-b63f-08076c4fc65e")
	providerPID      = uuid.MustParse("76e705bb-cd5a-49f3-99c2-cec1406c8e9e")
	providerCallback = urlMustParse("https://provider.dsp/")
	consumerCallback = urlMustParse("https://consumer.dsp/callback/")
)

//nolint:funlen
func TestTermination(t *testing.T) {
	offer := odrl.Offer{
		MessageOffer: odrl.MessageOffer{
			PolicyClass: odrl.PolicyClass{
				AbstractPolicyRule: odrl.AbstractPolicyRule{},
				ID:                 uuid.New().URN(),
			},
			Type:   "odrl:Offer",
			Target: target.URN(),
		},
	}

	logger := logging.New("error", true)
	ctx := ctxslog.Inject(t.Context(), logger)
	ctx, done := context.WithCancel(ctx)
	defer done()

	store, err := sqlite.New(ctx, true, false, "")
	assert.Nil(t, err)
	err = store.Migrate(ctx)
	assert.Nil(t, err)

	requester := &MockRequester{}

	mockProvider := mockdsrpc.NewMockProviderServiceClient(t)
	mockCService := mockdsrpc.NewMockContractServiceClient(t)

	reconciler := statemachine.NewReconciler(ctx, requester, store)
	reconciler.Run()

	for _, role := range []constants.DataspaceRole{
		constants.DataspaceConsumer,
		constants.DataspaceProvider,
	} {
		for _, state := range []contract.State{
			contract.States.REQUESTED,
			contract.States.OFFERED,
			contract.States.ACCEPTED,
			contract.States.AGREED,
			contract.States.VERIFIED,
		} {
			consumerPID := uuid.New()
			providerPID := uuid.New()
			negotiation := contract.New(
				ctx,
				providerPID, consumerPID,
				state, offer, providerCallback, consumerCallback, role, false, &dsrpc.RequesterInfo{
					AuthenticationStatus: dsrpc.AuthenticationStatus_AUTHENTICATION_STATUS_LOCAL_ORIGIN,
				})
			pid := consumerPID
			if role == constants.DataspaceProvider {
				pid = providerPID
			}
			mockCService.On("TerminationReceived", mock.Anything, &dsrpc.ContractServiceTerminationReceivedRequest{
				Pid:    pid.String(),
				Code:   "meh",
				Reason: []string{"test"},
			}).Return(&dsrpc.ContractServiceTerminationReceivedResponse{}, nil)
			consumerInit := statemachine.GetContractNegotiation(negotiation, mockProvider, mockCService, reconciler)
			msg := shared.ContractNegotiationTerminationMessage{
				Context:     shared.GetDSPContext(),
				Type:        "dspace:ContractNegotiationTerminationMessage",
				ProviderPID: providerPID.URN(),
				ConsumerPID: consumerPID.URN(),
				Code:        "meh",
				Reason: []shared.Multilanguage{
					{
						Language: "en",
						Value:    "test",
					},
				},
			}
			_, applyFunc, err := consumerInit.Recv(ctx, msg)
			assert.Nil(t, err)
			assert.Equal(t, contract.States.TERMINATED, consumerInit.GetState())
			err = applyFunc()
			assert.Nil(t, err)
		}
	}
}
