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
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// ParseUUIDUURN parses a UUID URN of the format `urn:uuid:<uuid>` and returns a uuid.UUID.
func ParseUUIDURN(uu string) (uuid.UUID, error) {
	u, err := url.Parse(uu)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("not a valid URL: %w", err)
	}
	if strings.ToLower(u.Scheme) != "urn" {
		return uuid.UUID{}, fmt.Errorf("wrong scheme `%s`, expected `urn`", u.Scheme)
	}
	parts := strings.Split(u.Opaque, ":")
	if len(parts) != 2 {
		return uuid.UUID{}, fmt.Errorf("not a valid URN: %s", uu)
	}
	if strings.ToLower(parts[0]) != "uuid" {
		return uuid.UUID{}, fmt.Errorf("wrong URN namespace: `%s`, expected `uuid`", parts[0])
	}
	return uuid.Parse(parts[1])
}
