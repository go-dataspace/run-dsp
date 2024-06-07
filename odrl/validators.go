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

package odrl

import (
	"slices"

	"github.com/go-playground/validator/v10"
)

func action(fl validator.FieldLevel) bool {
	states := []string{
		"odrl:delete",
		"odrl:execute",
		"cc:SourceCode",
		"odrl:anonymize",
		"odrl:extract",
		"odrl:read",
		"odrl:index",
		"odrl:compensate",
		"odrl:sell",
		"odrl:derive",
		"odrl:ensureExclusivity",
		"odrl:annotate",
		"cc:Reproduction",
		"odrl:translate",
		"odrl:include",
		"cc:DerivativeWorks",
		"cc:Distribution",
		"odrl:textToSpeech",
		"odrl:inform",
		"odrl:grantUse",
		"odrl:archive",
		"odrl:modify",
		"odrl:aggregate",
		"odrl:attribute",
		"odrl:nextPolicy",
		"odrl:digitize",
		"cc:Attribution",
		"odrl:install",
		"odrl:concurrentUse",
		"odrl:distribute",
		"odrl:synchronize",
		"odrl:move",
		"odrl:obtainConsent",
		"odrl:print",
		"cc:Notice",
		"odrl:give",
		"odrl:uninstall",
		"cc:Sharing",
		"odrl:reviewPolicy",
		"odrl:watermark",
		"odrl:play",
		"odrl:reproduce",
		"odrl:transform",
		"odrl:display",
		"odrl:stream",
		"cc:ShareAlike",
		"odrl:acceptTracking",
		"cc:CommericalUse",
		"odrl:present",
		"odrl:use",
	}
	return slices.Contains(states, fl.Field().String())
}

func leftOperand(fl validator.FieldLevel) bool {
	states := []string{
		"odrl:absolutePosition",
		"odrl:absoluteSize",
		"odrl:absoluteSpatialPosition",
		"odrl:absoluteTemporalPosition",
		"odrl:count",
		"odrl:dateTime",
		"odrl:delayPeriod",
		"odrl:deliveryChannel",
		"odrl:device",
		"odrl:elapsedTime",
		"odrl:event",
		"odrl:fileFormat",
		"odrl:industry",
		"odrl:language",
		"odrl:media",
		"odrl:meteredTime",
		"odrl:payAmount",
		"odrl:percentage",
		"odrl:product",
		"odrl:purpose",
		"odrl:recipient",
		"odrl:relativePosition",
		"odrl:relativeSize",
		"odrl:relativeSpatialPosition",
		"odrl:relativeTemporalPosition",
		"odrl:resolution",
		"odrl:spatial",
		"odrl:spatialCoordinates",
		"odrl:system",
		"odrl:systemDevice",
		"odrl:timeInterval",
		"odrl:unitOfCount",
		"odrl:version",
		"odrl:virtualLocation",
	}
	return slices.Contains(states, fl.Field().String())
}

func operator(fl validator.FieldLevel) bool {
	states := []string{
		"odrl:eq",
		"odrl:gt",
		"odrl:gteq",
		"odrl:hasPart",
		"odrl:isA",
		"odrl:isAllOf",
		"odrl:isAnyOf",
		"odrl:isNoneOf",
		"odrl:isPartOf",
		"odrl:lt",
		"odrl:term-lteq",
		"odrl:neq",
	}
	return slices.Contains(states, fl.Field().String())
}

// This registers all the validators of this package.
func RegisterValidators(v *validator.Validate) error {
	if err := v.RegisterValidation("odrl_action", action); err != nil {
		return err
	}
	if err := v.RegisterValidation("odrl_leftoperand", leftOperand); err != nil {
		return err
	}
	if err := v.RegisterValidation("odrl_operator", operator); err != nil {
		return err
	}
	return nil
}
