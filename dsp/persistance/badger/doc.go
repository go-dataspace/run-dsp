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

// Package badger contains an implementation of the persistance.StorageProvider interface.
// Badger is a pure-go key-value database, not unlike redis. It is made to be embeddable in
// go applications, and offers both on-disk and in-memory backends.
//
// This is intended to be the default storage backend for RUN-DSP.
package badger
