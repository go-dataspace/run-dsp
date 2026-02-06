// Copyright 2026 go-dataspace
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
//
//nolint:dupl
package sqlite_test

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go-dataspace.eu/run-dsp/dsp/constants"
	"go-dataspace.eu/run-dsp/dsp/contract"
	"go-dataspace.eu/run-dsp/dsp/persistence/backends/sqlite"
	contractopts "go-dataspace.eu/run-dsp/dsp/persistence/options/contract"
	"go-dataspace.eu/run-dsp/dsp/shared"
	"go-dataspace.eu/run-dsp/odrl"
)

type negotiationParams struct {
	name        string
	consumerPID uuid.UUID
	providerPID uuid.UUID
	state       contract.State
	offer       odrl.Offer
	agreement   *odrl.Agreement
	role        constants.DataspaceRole
	callback    *url.URL
	self        *url.URL
	autoAccept  bool
}

func (np negotiationParams) RolePID() uuid.UUID {
	switch np.role {
	case constants.DataspaceConsumer:
		return np.consumerPID
	case constants.DataspaceProvider:
		return np.providerPID
	default:
		panic(fmt.Sprintf("unexpected constants.DataspaceRole: %#v", np.role))
	}
}

var (
	targetID     = uuid.MustParse("98ba2d71-4be5-46aa-8147-0168ca27ffdc")
	agreementID  = uuid.MustParse("52641491-c3c6-4031-8435-2847151cf1ca")
	agreement2ID = uuid.MustParse("0465e8ad-f74a-40ab-bb71-11409e97a95f")

	odrlOffer = odrl.Offer{
		MessageOffer: odrl.MessageOffer{
			Type: "odrl:Offer",
			PolicyClass: odrl.PolicyClass{
				ID: uuid.MustParse("4e3770fd-63d5-4cd7-bb82-bca2ce0cf563").URN(),
				AbstractPolicyRule: odrl.AbstractPolicyRule{
					Assigner: "urn:blablabla",
				},
				Permission: []odrl.Permission{
					{
						Action: "odrl:use",
					},
				},
			},
			Target: targetID.URN(),
		},
	}
	odrlAgreement = odrl.Agreement{
		Type:        "odrl:Agreement",
		ID:          agreementID.URN(),
		Target:      targetID.URN(),
		Timestamp:   time.Date(1974, time.September, 9, 13, 14, 15, 0, time.UTC),
		PolicyClass: odrl.PolicyClass{},
	}

	odrlAgreement2 = odrl.Agreement{
		Type:        "odrl:Agreement",
		ID:          agreement2ID.URN(),
		Target:      targetID.URN(),
		Timestamp:   time.Date(1974, time.September, 9, 13, 14, 15, 0, time.UTC),
		PolicyClass: odrl.PolicyClass{},
	}

	callBack = shared.MustParseURL("http://example.com")
	selfURL  = shared.MustParseURL("http://example.org")

	negotiationInitConsumer = negotiationParams{
		name:        "init consumer",
		consumerPID: uuid.MustParse("64f4eba1-29af-4eb3-9e7e-330bd3c5d2ce"),
		providerPID: uuid.UUID{},
		state:       contract.States.INITIAL,
		offer:       odrlOffer,
		agreement:   nil,
		role:        constants.DataspaceConsumer,
		callback:    callBack,
		self:        selfURL,
		autoAccept:  false,
	}
	negotiationInitProvider = negotiationParams{
		name:        "init provider",
		consumerPID: uuid.UUID{},
		providerPID: uuid.MustParse("5a5377b7-ece1-4435-9137-c0cfe2e8bb91"),
		state:       contract.States.INITIAL,
		offer:       odrlOffer,
		agreement:   nil,
		role:        constants.DataspaceProvider,
		callback:    selfURL,
		self:        callBack,
		autoAccept:  false,
	}
	negotiationRequestedConsumer = negotiationParams{
		name:        "requested consumer",
		consumerPID: uuid.MustParse("543b6a04-82dd-4f9f-96ae-5e77c07179cd"),
		providerPID: uuid.MustParse("523874e9-b1ac-4f42-91bc-deeb5899d6d0"),
		state:       contract.States.REQUESTED,
		offer:       odrlOffer,
		agreement:   nil,
		role:        constants.DataspaceConsumer,
		callback:    callBack,
		self:        selfURL,
		autoAccept:  false,
	}
	negotiationRequestedProvider = negotiationParams{
		name:        "requested provider",
		consumerPID: uuid.MustParse("c871dd75-1bf2-4ba3-8504-cba3b1415214"),
		providerPID: uuid.MustParse("26a1b947-abbe-4119-832a-82b878695494"),
		state:       contract.States.REQUESTED,
		offer:       odrlOffer,
		agreement:   nil,
		role:        constants.DataspaceProvider,
		callback:    selfURL,
		self:        callBack,
		autoAccept:  false,
	}
	negotiationVerifiedConsumer = negotiationParams{
		name:        "verified consumer",
		consumerPID: uuid.MustParse("80d31028-d230-4408-b3ae-242abd89308d"),
		providerPID: uuid.MustParse("c33ebdd3-e821-4cbb-88cd-5bc0d91ae415"),
		state:       contract.States.VERIFIED,
		offer:       odrlOffer,
		agreement:   &odrlAgreement2,
		role:        constants.DataspaceConsumer,
		callback:    callBack,
		self:        selfURL,
		autoAccept:  false,
	}
	negotiationVerifiedProvider = negotiationParams{
		name:        "verified provider",
		consumerPID: uuid.MustParse("c374702b-830f-48fd-914d-c651ab79fcc7"),
		providerPID: uuid.MustParse("588db935-fbf3-442b-94f6-4ac9da025dcc"),
		state:       contract.States.VERIFIED,
		offer:       odrlOffer,
		agreement:   &odrlAgreement,
		role:        constants.DataspaceProvider,
		callback:    selfURL,
		self:        callBack,
		autoAccept:  false,
	}

	allNegotiations = []negotiationParams{
		negotiationInitConsumer,
		negotiationInitProvider,
		negotiationRequestedConsumer,
		negotiationRequestedProvider,
		negotiationVerifiedConsumer,
		negotiationVerifiedProvider,
	}
)

func negotiationsEqual(t *testing.T, expected, actual *contract.Negotiation) {
	t.Helper()
	r := require.New(t)
	r.Equal(expected.GetConsumerPID(), actual.GetConsumerPID())
	r.Equal(expected.GetProviderPID(), actual.GetProviderPID())
	r.Equal(expected.GetState(), actual.GetState())
	r.Equal(expected.GetOffer(), actual.GetOffer())
	r.Equal(expected.GetAgreement(), actual.GetAgreement())
	r.Equal(expected.GetCallback(), actual.GetCallback())
	r.Equal(expected.GetSelf(), actual.GetSelf())
	r.Equal(expected.GetRole(), actual.GetRole())
	r.Equal(expected.AutoAccept(), actual.AutoAccept())
	r.Equal(expected.GetTraceInfo(), actual.GetTraceInfo())
	r.Equal(expected.GetRequesterInfo(), actual.GetRequesterInfo())
}

func paramsToNegotiation(ctx context.Context, t *testing.T, params negotiationParams) *contract.Negotiation {
	t.Helper()
	neg := contract.New(
		ctx,
		params.providerPID,
		params.consumerPID,
		params.state,
		params.offer,
		params.callback,
		params.self,
		params.role,
		params.autoAccept,
		nil,
	)
	if params.agreement != nil {
		neg.SetAgreement(params.agreement)
	}
	return neg
}

func initialiseRecords(ctx context.Context, t *testing.T, p *sqlite.Provider) {
	t.Helper()
	r := require.New(t)
	for _, params := range allNegotiations {
		neg := paramsToNegotiation(ctx, t, params)
		if params.agreement != nil {
			r.Nil(p.PutAgreement(ctx, params.agreement))
		}
		r.Nil(p.PutContract(ctx, neg))
	}
	for _, params := range allRequests {
		req := paramsToRequest(ctx, t, params)
		r.Nil(p.PutTransfer(ctx, req))
	}
}

func TestPutAndGetNegotiations(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	initialiseRecords(ctx, t, p)

	for _, params := range allNegotiations {
		t.Run(fmt.Sprintf("getting %s", params.name), func(t *testing.T) {
			existing := paramsToNegotiation(ctx, t, params)
			new, err := p.GetContract(ctx, contractopts.WithRolePID(params.RolePID(), params.role))
			r.Nil(err)

			negotiationsEqual(t, existing, new)
		})
	}
	r.Nil(p.Close())
}

func TestPutAndMutateStateNegotiations(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	initialiseRecords(ctx, t, p)

	for _, params := range allNegotiations {
		t.Run(fmt.Sprintf("mutate %s", params.name), func(t *testing.T) {
			existing := paramsToNegotiation(ctx, t, params)
			r.Nil(existing.SetState(contract.States.TERMINATED))
			intermediate, err := p.GetContract(ctx,
				contractopts.WithRolePID(params.RolePID(), params.role),
				contractopts.WithRW(),
			)
			r.Nil(err)
			r.Nil(intermediate.SetState(contract.States.TERMINATED))
			r.Nil(p.PutContract(ctx, intermediate))
			new, err := p.GetContract(ctx, contractopts.WithRolePID(params.RolePID(), params.role))
			r.Nil(err)
			negotiationsEqual(t, existing, new)
		})
	}
	require.Nil(t, p.Close())
}

func TestPutDuplicateNegotiations(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	initialiseRecords(ctx, t, p)

	for _, params := range allNegotiations {
		t.Run(fmt.Sprintf("putting duplicate %s", params.name), func(t *testing.T) {
			existing := paramsToNegotiation(ctx, t, params)
			r.Error(p.PutContract(ctx, existing))
		})
	}
	r.Nil(p.Close())
}

func TestGetLockedNegotiations(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	for _, params := range allNegotiations {
		t.Run(fmt.Sprintf("getting locked duplicate %s", params.name), func(t *testing.T) {
			locked, err := p.GetContract(ctx, contractopts.WithRolePID(params.RolePID(), params.role), contractopts.WithRW())
			r.Nil(err)
			r.Equal(true, locked.GetLocked())
			r.Equal(false, locked.ReadOnly())
			_, err = p.GetContract(ctx, contractopts.WithRolePID(params.RolePID(), params.role), contractopts.WithRW())
			r.Error(err)
		})
	}
	r.Nil(p.Close())
}

func TestGetRequestedNegotiations(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	negotiations, err := p.GetContracts(ctx, contractopts.WithState(contract.States.REQUESTED))
	r.Nil(err)
	r.Equal(2, len(negotiations))
	r.True(slices.ContainsFunc(negotiations, func(n *contract.Negotiation) bool {
		return negotiationRequestedConsumer.consumerPID == n.GetConsumerPID()
	}))
	r.True(slices.ContainsFunc(negotiations, func(n *contract.Negotiation) bool {
		return negotiationRequestedProvider.providerPID == n.GetProviderPID()
	}))
	r.Nil(p.Close())
}

func TestGetCallbackNegotiations(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	negotiations, err := p.GetContracts(ctx, contractopts.WithCallback(selfURL))
	r.Nil(err)
	r.Equal(3, len(negotiations))
	for _, neg := range negotiations {
		r.Equal(selfURL, neg.GetCallback())
	}
	r.Nil(p.Close())
}

func TestGetRoleNegotiations(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	negotiations, err := p.GetContracts(ctx, contractopts.WithRole(constants.DataspaceConsumer))
	r.Nil(err)
	r.Equal(3, len(negotiations))
	for _, neg := range negotiations {
		r.Equal(constants.DataspaceConsumer, neg.GetRole())
	}
	r.Nil(p.Close())
}
