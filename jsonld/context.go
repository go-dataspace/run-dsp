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

// Package jsonld contains utility types and functions to handle JSON-LD files.
package jsonld

import (
	"encoding/json"
	"fmt"
)

// ContextEntry is a JSON-LD context entry.
type ContextEntry struct {
	ID   string `json:"@id,omitempty"`
	Type string `json:"@type,omitempty"`
}

// UnmarshalJSON unmarshals a context entry. It first tries to unmarshal an entry as an object,
// and if that fails, it will try to unmarshal it as a string. If that succeeds, it will
// assign that string to the `ID` field.
func (ce *ContextEntry) UnmarshalJSON(data []byte) error {
	var entry struct {
		ID   string `json:"@id,omitempty"`
		Type string `json:"@type,omitempty"`
	}
	if err := json.Unmarshal(data, &entry); err == nil {
		ce.ID = entry.ID
		ce.Type = entry.Type
		return nil
	}

	var id string
	if err := json.Unmarshal(data, &id); err == nil {
		ce.ID = id
		return nil
	}
	return fmt.Errorf("Couldn't unmarshal ContextEntry: %s", data)
}

// MarshalJSON will marshal the ContextEntry as an object if Type is not empty, and
// as a string containing just the ID if Type is empty.
func (ce *ContextEntry) MarshalJSON() ([]byte, error) {
	if ce.Type != "" {
		return json.Marshal(struct {
			ID   string `json:"@id,omitempty"`
			Type string `json:"@type,omitempty"`
		}{
			ID:   ce.ID,
			Type: ce.Type,
		})
	}

	return json.Marshal(ce.ID)
}

// Context is a JSON-LD @context entry. In JSON-LD this can be a string, a list of strings, a map
// containing either strings or objects.
type Context struct {
	rootContexts  []ContextEntry
	namedContexts map[string]ContextEntry
}

// UnmarshalJSON will first try to unmarshal the context as a map of ContextEntry structs,
// if that fails, it will try to unmarshal it as a list of strings, and if that fails, as
// a single string.
func (c *Context) UnmarshalJSON(data []byte) error {
	var nc map[string]ContextEntry
	if err := json.Unmarshal(data, &nc); err == nil {
		c.namedContexts = nc
		return nil
	}
	rootContexts := make([]ContextEntry, 0)
	var lc []string
	if err := json.Unmarshal(data, &lc); err == nil {
		for _, id := range lc {
			rootContexts = append(rootContexts, ContextEntry{ID: id})
		}
		c.rootContexts = rootContexts
		return nil
	}
	var sc string
	if err := json.Unmarshal(data, &sc); err == nil {
		rootContexts = append(rootContexts, ContextEntry{ID: sc})
		c.rootContexts = rootContexts
		return nil
	}
	return fmt.Errorf("Couldn't unmarshal Context: %s", data)
}

// MarshalJSON will first check if namedContext are available, and marshal those if they are.
// If they are not, it will check if there's only a single rootContext and marshal that as a
// string, else it will marshal the entire rootContexts list.
func (c *Context) MarshalJSON() ([]byte, error) {
	if len(c.namedContexts) == 0 {
		return json.Marshal(c.namedContexts)
	}
	if len(c.rootContexts) == 1 {
		return json.Marshal(c.rootContexts[0])
	}
	return json.Marshal(c.rootContexts)
}

// GetContextsFor looks up the namespace/element by name and return the relevant contexts for it.
// If there are any named contexts, it returns those, else it returns the root contexts.
func (c *Context) GetContextsFor(ns string) []ContextEntry {
	if context, found := c.namedContexts[ns]; found {
		return []ContextEntry{context}
	}
	return c.rootContexts
}

// GetRootContexts returns the root contexts, this can be empty if there are named contexts.
func (c *Context) GetRootContexts() []ContextEntry {
	return c.rootContexts
}

// NewRootContext creates a new context from the given list. Do note that any `Type` properties
// will be lost.
func NewRootContext(c []ContextEntry) Context {
	return Context{
		rootContexts:  c,
		namedContexts: map[string]ContextEntry{},
	}
}

// NewNamedContext creates a new context from the given map.
func NewNamedContext(c map[string]ContextEntry) Context {
	return Context{
		rootContexts:  make([]ContextEntry, 0),
		namedContexts: c,
	}
}
