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

// Package shared contains DSP types and interfaces that both the dsp package and
// packages it imports will use.
package shared

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// File represents a file and what we can use according to the dcat distribution type.
type File struct {
	ID       uuid.UUID
	Name     string
	Modified string
	Format   string
	FullPath string
}

// CitizenData is the citizen data we will pass to the functions.
type CitizenData struct {
	FirstName string
	LastName  string
	BirthDate time.Time
}

type PublishInfo struct {
	URL   string
	Token string
}

// FileProvider exposes functions to get catalog information.
type FileProvider interface {
	GetFileSet(ctx context.Context, citizenData *CitizenData) ([]*File, error)
	GetFile(ctx context.Context, citizenData *CitizenData, id uuid.UUID) (*File, error)
	PublishFile(ctx context.Context, citizenData *CitizenData, fileID, processID uuid.UUID) (PublishInfo, error)
	UnpublishFile(ctx context.Context, processID uuid.UUID)
}
