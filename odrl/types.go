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

// Package odrl contains ODRL code
package odrl

import "time"

//nolint:lll
// This is for now a partial port of this JSON schema:
// https://international-data-spaces-association.github.io/ids-specification/2024-1/negotiation/message/schema/contract-schema.json

// Offer is an ODRL offer.
type Offer struct {
	MessageOffer
}

// MessageOffer is an ODRL MessageOffer.
type MessageOffer struct {
	PolicyClass
	Type   string `json:"@type" validate:"required,eq=odrl:Offer"`
	Target string `json:"odrl:target" validate:"required"`
}

// PolicyClass is an ODRL PolicyClass.
type PolicyClass struct {
	AbstractPolicyRule
	ID          string       `json:"@id" validate:"required"`
	ProviderID  string       `json:"dspace:providerId,omitempty"` // Got from an example, not in standard.
	Profile     []Reference  `json:"odrl:profile,omitempty" validate:"dive"`
	Permission  []Permission `json:"odrl:permission,omitempty" validate:"dive"`
	Obligation  []Duty       `json:"odrl:obligation,omitempty" validate:"dive"`
	Prohibition []any        `json:"odrl:prohibition"` // Spec for this was missing but is required, even if empty.
}

// AbstractPolicyRule defines an ODRL abstract policy rule.
type AbstractPolicyRule struct {
	Assigner string `json:"odrl:assigner,omitempty"`
	Assignee string `json:"odrl:assignee,omitempty"`
}

// Reference is a reference.
type Reference struct {
	ID string `json:"@id,omitempty" validate:"required"`
}

// Permission is a permisson entry.
type Permission struct {
	AbstractPolicyRule
	Action     string       `json:"action" validate:"required,odrl_action"`
	Constraint []Constraint `json:"constraint,omitempty" validate:"gte=1,dive"`
	Duty       Duty         `json:"duty,omitempty"`
}

// Duty is an ODRL duty.
type Duty struct {
	AbstractPolicyRule
	ID         string       `json:"@id,omitempty"`
	Action     string       `json:"action,omitempty" validate:"required,odrl_action"`
	Constraint []Constraint `json:"constraint,omitempty" validate:"gte=1,dive"`
}

// Constraint is an ODRL constraint.
type Constraint struct {
	RightOperand          string    `json:"odrl:rightOperand"`
	RightOperandReference Reference `json:"odrl:rightOperandReference,omitempty"`
	LeftOperand           string    `json:"odrl:leftOperand" validate:"odrl_leftoperand"`
	Operator              string    `json:"odrl:operator" validate:"odrl_operator"` // TODO: implment custom verifier.
}

// Agreement is an ODRL agreement.
type Agreement struct {
	PolicyClass
	Type      string    `json:"@type" validate:"required,eq=odrl:Agreement"`
	ID        string    `json:"@id" validate:"required"`
	Target    string    `json:"odrl:target" validate:"required"`
	Timestamp time.Time `json:"dspace:timestamp"`
}
