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

package shared

import (
	"fmt"
	"strings"

	"github.com/go-dataspace/run-dsp/oid"
	"github.com/google/uuid"
)

// IDtoURN generates the URN for the ID. Right now we only support preformatted URNs,
// UUIDs, and OIDs.
func IDToURN(s string) string {
	// If the string starts with urn:, we assume the string is already an URN.
	if strings.HasPrefix(strings.ToLower(s), "urn:") {
		return s
	}

	// Check if we are dealing with a UUID.
	if u, err := uuid.Parse(s); err == nil {
		return u.URN()
	}

	// If not, maybe an OID?
	if o, err := oid.Parse(s); err == nil {
		return o.URN()
	}

	// If still unknown, return an unknown URN.
	return fmt.Sprintf("urn:unknown:%s", s)
}

// URNtoRawID strips the URN part and returns the ID without any metadata.
// This function only works on URNs with 3 parts like the uuid or oid URNs.
// This function returns an error when we can't properly split it.
// TODO: Verify that all providers support URNs so that we can remove this function.
func URNtoRawID(s string) (string, error) {
	// If the ID doesn't start with "urn:", we just return it.
	if !strings.HasPrefix(strings.ToLower(s), "urn:") {
		return s, nil
	}

	parts := strings.SplitN(s, ":", 3)
	// If we don't get the right amount of parts, we return the full string, despite it most likely
	// won't be the result we want. This should be exceedingly rare, and we might want to panic here
	// instead to make the problem very obvious.
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed URN: %s", s)
	}

	return parts[2], nil
}
