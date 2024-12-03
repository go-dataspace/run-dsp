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

package statemachine

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"time"

	"github.com/go-dataspace/run-dsp/dsp/constants"
	"github.com/go-dataspace/run-dsp/dsp/contract"
	"github.com/go-dataspace/run-dsp/dsp/shared"
	"github.com/go-dataspace/run-dsp/logging"
	"github.com/go-dataspace/run-dsp/odrl"
	"github.com/google/uuid"
)

func cloneURL(u *url.URL) *url.URL {
	nu, err := url.Parse(u.String())
	if err != nil {
		panic(fmt.Sprintf("Couldn't clone url %s: %s", u.String(), err.Error()))
	}
	return nu
}

func makeContractRequestFunction(
	ctx context.Context,
	c *contract.Negotiation,
	cu *url.URL,
	reqBody []byte,
	destinationState contract.State,
	reconciler *Reconciler,
) func() {
	var id uuid.UUID
	if c.GetRole() == constants.DataspaceConsumer {
		id = c.GetConsumerPID()
	} else {
		id = c.GetProviderPID()
	}
	return makeRequestFunction(
		ctx,
		cu,
		reqBody,
		id,
		c.GetRole(),
		destinationState.String(),
		ReconciliationContract,
		reconciler,
	)
}

func makeRequestFunction(
	ctx context.Context,
	cu *url.URL,
	reqBody []byte,
	id uuid.UUID,
	role constants.DataspaceRole,
	destinationState string,
	recType ReconciliationType,
	reconciler *Reconciler,
) func() {
	return func() {
		reconciler.Add(ReconciliationEntry{
			EntityID:    id,
			Type:        recType,
			Role:        role,
			TargetState: destinationState,
			Method:      "POST",
			URL:         cu,
			Body:        reqBody,
			Context:     ctx,
		})
	}
}

//nolint:dupl
func sendContractRequest(ctx context.Context, r *Reconciler, c *contract.Negotiation) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractRequest")
	contractRequest := shared.ContractRequestMessage{
		Context:         shared.GetDSPContext(),
		Type:            "dspace:ContractRequestMessage",
		ConsumerPID:     c.GetConsumerPID().URN(),
		Offer:           c.GetOffer().MessageOffer,
		CallbackAddress: c.GetSelf().String(),
	}

	// If we have a providerPID we set it.
	if c.GetProviderPID() != emptyUUID {
		contractRequest.ProviderPID = c.GetProviderPID().URN()
	}
	reqBody, err := shared.ValidateAndMarshal(ctx, contractRequest)
	if err != nil {
		logger.Error("Could not validate contract request", "err", err)
		return func() {}, fmt.Errorf("could not validate contract request: %w", err)
	}

	cu := cloneURL(c.GetCallback())
	// Set the desired URL depending on if the provider PID is known.
	if c.GetProviderPID() != emptyUUID {
		cu.Path = path.Join(cu.Path, "negotiations", c.GetProviderPID().String(), "request")
	} else {
		cu.Path = path.Join(cu.Path, "negotiations", "request")
	}

	return makeContractRequestFunction(
		ctx,
		c,
		cu,
		reqBody,
		contract.States.REQUESTED,
		r,
	), nil
}

//nolint:dupl
func sendContractOffer(ctx context.Context, r *Reconciler, c *contract.Negotiation) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractOffer")
	contractOffer := shared.ContractOfferMessage{
		Context:         shared.GetDSPContext(),
		Type:            "dspace:ContractOfferMessage",
		ProviderPID:     c.GetProviderPID().URN(),
		Offer:           c.GetOffer().MessageOffer,
		CallbackAddress: c.GetSelf().String(),
	}

	if c.GetConsumerPID() != emptyUUID {
		contractOffer.ConsumerPID = c.GetConsumerPID().URN()
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, contractOffer)
	if err != nil {
		logger.Error("Could not validate contract request", "err", err)
		return func() {}, fmt.Errorf("could not validate contract request: %w", err)
	}

	cu := cloneURL(c.GetCallback())

	if c.GetConsumerPID() != emptyUUID {
		cu.Path = path.Join(cu.Path, "negotiations", c.GetConsumerPID().String(), "offers")
	} else {
		// TODO: As this is the only negotiations callback that uses the root of the API, it
		// will need some trickery to switch the callback in the contract.
		cu.Path = path.Join(cu.Path, "negotiations", "offers")
	}

	return makeContractRequestFunction(
		ctx,
		c,
		cu,
		reqBody,
		contract.States.OFFERED,
		r,
	), nil
}

func sendContractAgreement(ctx context.Context, r *Reconciler, c *contract.Negotiation) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractAgreement")
	c.SetAgreement(&odrl.Agreement{
		PolicyClass: odrl.PolicyClass{},
		Type:        "odrl:Agreement",
		ID:          uuid.New().URN(),
		Target:      c.GetOffer().Target,
		Timestamp:   time.Now(),
	})
	contractAgreement := shared.ContractAgreementMessage{
		Context:         shared.GetDSPContext(),
		Type:            "dspace:ContractAgreementMessage",
		ProviderPID:     c.GetProviderPID().URN(),
		ConsumerPID:     c.GetConsumerPID().URN(),
		Agreement:       *c.GetAgreement(),
		CallbackAddress: c.GetSelf().String(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, contractAgreement)
	if err != nil {
		logger.Error("Couldn't validate contract agreement", "err", err)
		return func() {}, fmt.Errorf("couldn't validate contract agreement: %w", err)
	}
	cu := cloneURL(c.GetCallback())
	cu.Path = path.Join(cu.Path, "negotiations", c.GetConsumerPID().String(), "agreement")

	return makeContractRequestFunction(
		ctx,
		c,
		cu,
		reqBody,
		contract.States.AGREED,
		r,
	), nil
}

func sendContractEvent(
	ctx context.Context, r *Reconciler, c *contract.Negotiation, pid uuid.UUID, state contract.State,
) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractEvent")
	contractEvent := shared.ContractNegotiationEventMessage{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:ContractNegotiationEventMessage",
		ProviderPID: c.GetProviderPID().URN(),
		ConsumerPID: c.GetConsumerPID().URN(),
		EventType:   state.String(),
	}
	reqBody, err := shared.ValidateAndMarshal(ctx, contractEvent)
	if err != nil {
		logger.Error("Couldn't validate contract event", "err", err)
		return func() {}, fmt.Errorf("couldn't validate contract event: %w", err)
	}
	cu := cloneURL(c.GetCallback())
	cu.Path = path.Join(cu.Path, "negotiations", pid.String(), "events")

	return makeContractRequestFunction(
		ctx,
		c,
		cu,
		reqBody,
		state,
		r,
	), nil
}

func sendContractVerification(ctx context.Context, r *Reconciler, c *contract.Negotiation) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractVerification")
	contractVerification := shared.ContractAgreementVerificationMessage{
		Context:     shared.GetDSPContext(),
		Type:        "dspace:ContractAgreementVerificationMessage",
		ProviderPID: c.GetProviderPID().URN(),
		ConsumerPID: c.GetConsumerPID().URN(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, contractVerification)
	if err != nil {
		logger.Error("Couldn't validate contract verification", "err", err)
		return func() {}, fmt.Errorf("couldn't validate contract verification: %w", err)
	}

	cu := cloneURL(c.GetCallback())
	cu.Path = path.Join(cu.Path, "negotiations", c.GetProviderPID().String(), "agreement", "verification")

	return makeContractRequestFunction(
		ctx,
		c,
		cu,
		reqBody,
		contract.States.VERIFIED,
		r,
	), nil
}
