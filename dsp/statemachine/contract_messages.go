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

// TODO: dedup this and the other request function.
//
//nolint:dupl
func makeContractRequestFunction(
	ctx context.Context,
	requester Requester,
	cu *url.URL,
	reqBody []byte,
	c *Contract,
	a Archiver,
	destinationState ContractState,
) func() {
	return func() {
		logger := logging.Extract(ctx)
		logger.Debug("Sending request")
		respBody, err := requester.SendHTTPRequest(ctx, "POST", cu, reqBody)
		if err != nil {
			logger.Error("Could not send request", "error", err)
			return
		}

		if len(respBody) > 0 {
			cneg, err := shared.UnmarshalAndValidate(ctx, respBody, shared.ContractNegotiation{})
			if err != nil {
				logger.Error("Could not parse response", "error", err)
				return
			}

			state, err := ParseContractState(cneg.State)
			if err != nil {
				logger.Error("Invalid state returned", "state", cneg.State)
				return
			}

			if state != destinationState {
				logger.Error("Invalid state returned", "state", state)
				return
			}
		}
		err = c.SetState(destinationState)
		if err != nil {
			logger.Error("Tried to set invalid state", "error", err)
			return
		}
		if c.role == ContractConsumer {
			err = a.PutConsumerContract(ctx, c)
		} else {
			err = a.PutProviderContract(ctx, c)
		}
		if err != nil {
			logger.Error("Could not store  contract", "error", err)
			return
		}
	}
}

//nolint:dupl
func sendContractRequest(ctx context.Context, r Requester, c *Contract, a Archiver) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractRequest")
	contractRequest := shared.ContractRequestMessage{
		Context:         dspaceContext,
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
		logger.Error("Could not validate contract request", "error", err)
		return func() {}, fmt.Errorf("could not validate contract request: %w", err)
	}

	cu := cloneURL(c.GetCallback())
	// Set the desired URL depending on if the provider PID is known.
	if c.GetProviderPID() != emptyUUID {
		cu.Path = path.Join(cu.Path, "negotiations", c.GetProviderPID().String(), "request")
	} else {
		cu.Path = path.Join(cu.Path, "negotiations", "request")
	}

	return makeContractRequestFunction(ctx, r, cu, reqBody, c, a, ContractStates.REQUESTED), nil
}

//nolint:dupl
func sendContractOffer(ctx context.Context, r Requester, c *Contract, a Archiver) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractOffer")
	contractOffer := shared.ContractOfferMessage{
		Context:         dspaceContext,
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
		logger.Error("Could not validate contract request", "error", err)
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

	return makeContractRequestFunction(ctx, r, cu, reqBody, c, a, ContractStates.OFFERED), nil
}

func sendContractAgreement(ctx context.Context, r Requester, c *Contract, a Archiver) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractAgreement")
	c.agreement = odrl.Agreement{
		PolicyClass: odrl.PolicyClass{},
		Type:        "odrl:Agreement",
		ID:          uuid.New().URN(),
		Target:      c.GetOffer().Target,
		Timestamp:   time.Now(),
	}
	contractAgreement := shared.ContractAgreementMessage{
		Context:         dspaceContext,
		Type:            "dspace:ContractAgreementMessage",
		ProviderPID:     c.GetProviderPID().URN(),
		ConsumerPID:     c.GetConsumerPID().URN(),
		Agreement:       c.GetAgreement(),
		CallbackAddress: c.GetSelf().String(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, contractAgreement)
	if err != nil {
		logger.Error("Couldn't validate contract agreement", "error", err)
		return func() {}, fmt.Errorf("couldn't validate contract agreement: %w", err)
	}
	cu := cloneURL(c.GetCallback())
	cu.Path = path.Join(cu.Path, "negotiations", c.GetConsumerPID().String(), "agreement")

	return makeContractRequestFunction(ctx, r, cu, reqBody, c, a, ContractStates.AGREED), nil
}

func sendContractEvent(
	ctx context.Context, r Requester, c *Contract, a Archiver, pid uuid.UUID, state ContractState,
) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractEvent")
	contractEvent := shared.ContractNegotiationEventMessage{
		Context:     dspaceContext,
		Type:        "dspace:ContractNegotiationEventMessage",
		ProviderPID: c.GetProviderPID().URN(),
		ConsumerPID: c.GetConsumerPID().URN(),
		EventType:   state.String(),
	}
	reqBody, err := shared.ValidateAndMarshal(ctx, contractEvent)
	if err != nil {
		logger.Error("Couldn't validate contract event", "error", err)
		return func() {}, fmt.Errorf("couldn't validate contract event: %w", err)
	}
	cu := cloneURL(c.GetCallback())
	cu.Path = path.Join(cu.Path, "negotiations", pid.String(), "events")

	return makeContractRequestFunction(ctx, r, cu, reqBody, c, a, state), nil
}

func sendContractVerification(ctx context.Context, r Requester, c *Contract, a Archiver) (func(), error) {
	ctx, logger := logging.InjectLabels(ctx, "operation", "sendContractVerification")
	contractVerification := shared.ContractAgreementVerificationMessage{
		Context:     dspaceContext,
		Type:        "dspace:ContractAgreementVerificationMessage",
		ProviderPID: c.GetProviderPID().URN(),
		ConsumerPID: c.GetConsumerPID().URN(),
	}

	reqBody, err := shared.ValidateAndMarshal(ctx, contractVerification)
	if err != nil {
		logger.Error("Couldn't validate contract verification", "error", err)
		return func() {}, fmt.Errorf("couldn't validate contract verification: %w", err)
	}

	cu := cloneURL(c.GetCallback())
	cu.Path = path.Join(cu.Path, "negotiations", c.GetProviderPID().String(), "agreement", "verification")

	return makeContractRequestFunction(ctx, r, cu, reqBody, c, a, ContractStates.VERIFIED), nil
}
