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

	"github.com/go-dataspace/run-dsp/dsp/constants"
	"github.com/go-dataspace/run-dsp/dsp/contract"
	"github.com/go-dataspace/run-dsp/dsp/persistence/badger"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/dsp/statemachine"
	"github.com/go-dataspace/run-dsp/logging"
	mockprovider "github.com/go-dataspace/run-dsp/mocks/github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha1"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
	t.Parallel()

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

	logger := logging.NewJSON("error", true)
	ctx := logging.Inject(context.Background(), logger)
	ctx, done := context.WithCancel(ctx)
	defer done()

	store, err := badger.New(ctx, true, "")
	assert.Nil(t, err)

	requester := &MockRequester{}

	mockProvider := mockprovider.NewMockProviderServiceClient(t)

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
				providerPID, consumerPID,
				state, offer, providerCallback, consumerCallback, role)
			ctx, consumerInit := statemachine.GetContractNegotiation(ctx, negotiation, mockProvider, reconciler)
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
			ctx, next, err := consumerInit.Recv(ctx, msg)
			assert.IsType(t, &statemachine.ContractNegotiationTerminated{}, next)
			assert.Nil(t, err)
			_, err = next.Send(ctx)
			assert.Nil(t, err)
			assert.Equal(t, contract.States.TERMINATED, next.GetContract().GetState())
		}
	}
}
