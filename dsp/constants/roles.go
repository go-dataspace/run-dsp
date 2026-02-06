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

package constants

import "fmt"

// DataspaceRole signifies what role in a dataspace exchange this is.
type DataspaceRole uint8

const (
	DataspaceConsumer DataspaceRole = iota
	DataspaceProvider
)

func ParseRole(s string) (DataspaceRole, error) {
	switch s {
	case "Consumer":
		return DataspaceConsumer, nil
	case "Provider":
		return DataspaceProvider, nil
	default:
		return 255, fmt.Errorf("not a valid role: %s", s)
	}
}

func (r DataspaceRole) String() string {
	switch r {
	case DataspaceConsumer:
		return "Consumer"
	case DataspaceProvider:
		return "Provider"
	default:
		panic(fmt.Sprintf("unexpected constants.DataspaceRole: %#v", r))
	}
}
