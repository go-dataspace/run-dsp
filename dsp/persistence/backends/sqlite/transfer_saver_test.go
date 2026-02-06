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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go-dataspace.eu/run-dsp/dsp/constants"
	transferopts "go-dataspace.eu/run-dsp/dsp/persistence/options/transfer"
	"go-dataspace.eu/run-dsp/dsp/transfer"
	"go-dataspace.eu/run-dsp/odrl"
	dsrpc "go-dataspace.eu/run-dsrpc/gen/go/dsp/v1alpha2"
)

type transferParams struct {
	name        string
	consumerPID uuid.UUID
	providerPID uuid.UUID
	state       transfer.State
	format      string
	agreement   odrl.Agreement
	publishInfo *dsrpc.PublishInfo
	role        constants.DataspaceRole
	callback    *url.URL
	self        *url.URL
}

func (tp transferParams) RolePID() uuid.UUID {
	switch tp.role {
	case constants.DataspaceConsumer:
		return tp.consumerPID
	case constants.DataspaceProvider:
		return tp.providerPID
	default:
		panic(fmt.Sprintf("unexpected constants.DataspaceRole: %#v", tp.role))
	}
}

var (
	requestStartedConsumer = transferParams{
		name:        "started consumer",
		consumerPID: uuid.MustParse("64f4eba1-29af-4eb3-9e7e-330bd3c5d2ce"),
		providerPID: uuid.MustParse("c4acb600-4e57-490f-8667-ea4d978f380e"),
		state:       transfer.States.STARTED,
		agreement:   odrlAgreement,
		role:        constants.DataspaceConsumer,
		callback:    callBack,
		self:        selfURL,
		format:      "HTTP:PULL",
		publishInfo: nil,
	}
	requestStartedProvider = transferParams{
		name:        "started provider",
		consumerPID: uuid.MustParse("c761ec71-7a34-4ba9-858c-d53919484496"),
		providerPID: uuid.MustParse("eb2b2e2f-f3a1-4690-834c-9686c2fcb779"),
		state:       transfer.States.STARTED,
		agreement:   odrlAgreement,
		role:        constants.DataspaceProvider,
		callback:    selfURL,
		self:        callBack,
		format:      "HTTP:PUSH",
		publishInfo: &dsrpc.PublishInfo{
			Url:                "https://example.org",
			AuthenticationType: dsrpc.AuthenticationType_AUTHENTICATION_TYPE_BEARER,
			Username:           "",
			Password:           "some_bearer",
		},
	}
	requestSuspendedConsumer = transferParams{
		name:        "completed consumer",
		consumerPID: uuid.MustParse("6e9547d3-342c-4ac5-88d7-176924258199"),
		providerPID: uuid.MustParse("8e9c77c1-8a7e-4250-b19b-cabf4267e55f"),
		state:       transfer.States.SUSPENDED,
		agreement:   odrlAgreement,
		role:        constants.DataspaceConsumer,
		callback:    callBack,
		self:        selfURL,
		format:      "HTTP:PULL",
		publishInfo: nil,
	}
	requestSuspendedProvider = transferParams{
		name:        "completed provider",
		consumerPID: uuid.MustParse("ec1db2b4-f342-44ea-ac06-87d03d11ae32"),
		providerPID: uuid.MustParse("33e68029-8790-4050-ae69-3be362edfaf7"),
		state:       transfer.States.SUSPENDED,
		agreement:   odrlAgreement,
		role:        constants.DataspaceProvider,
		callback:    selfURL,
		self:        callBack,
		format:      "HTTP:PUSH",
		publishInfo: &dsrpc.PublishInfo{
			Url:                "https://example.org",
			AuthenticationType: dsrpc.AuthenticationType_AUTHENTICATION_TYPE_BEARER,
			Username:           "",
			Password:           "some_bearer",
		},
	}

	allRequests = []transferParams{
		requestStartedConsumer,
		requestStartedProvider,
		requestSuspendedConsumer,
		requestSuspendedProvider,
	}
)

func requestsEqual(t *testing.T, expected, actual *transfer.Request) {
	t.Helper()
	r := require.New(t)
	r.Equal(expected.GetConsumerPID(), actual.GetConsumerPID())
	r.Equal(expected.GetProviderPID(), actual.GetProviderPID())
	r.Equal(expected.GetState(), actual.GetState())
	r.Equal(expected.GetTransferDirection(), actual.GetTransferDirection())
	r.Equal(expected.GetAgreementID(), actual.GetAgreementID())
	r.Equal(expected.GetCallback(), actual.GetCallback())
	r.Equal(expected.GetSelf(), actual.GetSelf())
	r.Equal(expected.GetRole(), actual.GetRole())
	r.Equal(expected.GetPublishInfo(), actual.GetPublishInfo())
	r.Equal(expected.GetTraceInfo(), actual.GetTraceInfo())
	r.Equal(expected.GetRequesterInfo(), actual.GetRequesterInfo())
}

func paramsToRequest(ctx context.Context, t *testing.T, params transferParams) *transfer.Request {
	t.Helper()
	req := transfer.New(
		ctx,
		params.consumerPID,
		&params.agreement,
		params.format,
		params.callback,
		params.self,
		params.role,
		params.state,
		params.publishInfo,
		nil,
	)
	req.SetProviderPID(params.providerPID)
	return req
}

func TestPutAndGetRequests(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	initialiseRecords(ctx, t, p)

	for _, params := range allRequests {
		t.Run(fmt.Sprintf("getting %s", params.name), func(t *testing.T) {
			existing := paramsToRequest(ctx, t, params)
			new, err := p.GetTransfer(ctx, transferopts.WithRolePID(params.RolePID(), params.role))
			r.Nil(err)

			requestsEqual(t, existing, new)
		})
	}
	r.Nil(p.Close())
}

func TestPutAndMutateStateRequests(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	initialiseRecords(ctx, t, p)

	for _, params := range allRequests {
		t.Run(fmt.Sprintf("mutate %s", params.name), func(t *testing.T) {
			existing := paramsToRequest(ctx, t, params)
			r.Nil(existing.SetState(transfer.States.TERMINATED))
			intermediate, err := p.GetTransfer(ctx,
				transferopts.WithRolePID(params.RolePID(), params.role),
				transferopts.WithRW(),
			)
			r.Nil(err)
			r.Nil(intermediate.SetState(transfer.States.TERMINATED))
			r.Nil(p.PutTransfer(ctx, intermediate))
			new, err := p.GetTransfer(ctx, transferopts.WithRolePID(params.RolePID(), params.role))
			r.Nil(err)
			requestsEqual(t, existing, new)
		})
	}
	require.Nil(t, p.Close())
}

func TestPutDuplicateRequests(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	initialiseRecords(ctx, t, p)

	for _, params := range allRequests {
		t.Run(fmt.Sprintf("putting duplicate %s", params.name), func(t *testing.T) {
			existing := paramsToRequest(ctx, t, params)
			r.Error(p.PutTransfer(ctx, existing))
		})
	}
	r.Nil(p.Close())
}

func TestLockedRequests(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	for _, params := range allRequests {
		t.Run(fmt.Sprintf("getting locked duplicate %s", params.name), func(t *testing.T) {
			locked, err := p.GetTransfer(ctx, transferopts.WithRolePID(params.RolePID(), params.role), transferopts.WithRW())
			r.Nil(err)
			r.Equal(true, locked.GetLocked())
			r.Equal(false, locked.ReadOnly())
			_, err = p.GetTransfer(ctx, transferopts.WithRolePID(params.RolePID(), params.role), transferopts.WithRW())
			r.Error(err)
		})
	}
	r.Nil(p.Close())
}

func TestGetStateRequests(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	Requests, err := p.GetTransfers(ctx, transferopts.WithState(transfer.States.STARTED))
	r.Nil(err)
	r.Equal(2, len(Requests))
	r.True(slices.ContainsFunc(Requests, func(n *transfer.Request) bool {
		return requestStartedConsumer.consumerPID == n.GetConsumerPID()
	}))
	r.True(slices.ContainsFunc(Requests, func(n *transfer.Request) bool {
		return requestStartedProvider.providerPID == n.GetProviderPID()
	}))
	r.Nil(p.Close())
}

func TestGetCallbackRequests(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	Requests, err := p.GetTransfers(ctx, transferopts.WithCallback(selfURL))
	r.Nil(err)
	r.Equal(2, len(Requests))
	for _, neg := range Requests {
		r.Equal(selfURL, neg.GetCallback())
	}
	r.Nil(p.Close())
}

func TestGetRoleRequests(t *testing.T) {
	r := require.New(t)
	ctx, p := setupEnv(t, false)
	p.SetLockTimeout(1)
	initialiseRecords(ctx, t, p)

	Requests, err := p.GetTransfers(ctx, transferopts.WithRole(constants.DataspaceConsumer))
	r.Nil(err)
	r.Equal(2, len(Requests))
	for _, neg := range Requests {
		r.Equal(constants.DataspaceConsumer, neg.GetRole())
	}
	r.Nil(p.Close())
}
