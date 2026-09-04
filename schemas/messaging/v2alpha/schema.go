// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package v2alpha provides validation of ETOS SSEv2 messaging events against the
// canonical JSON schema, which is embedded from this package so that there is a
// single source of truth for the event protocol. It is intended to be imported by
// other services (such as etos-api) that need to validate events.
package v2alpha

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed events.schema.json
var schemaBytes []byte

// schema is the compiled events schema, compiled once at package initialization.
var schema = mustCompileSchema()

// mustCompileSchema compiles the embedded events schema and panics if it cannot
// be compiled, since that indicates a packaging error that must never ship.
func mustCompileSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		panic(fmt.Sprintf("messaging schema: could not parse embedded schema: %v", err))
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("events.schema.json", doc); err != nil {
		panic(fmt.Sprintf("messaging schema: could not add embedded schema: %v", err))
	}
	compiled, err := compiler.Compile("events.schema.json")
	if err != nil {
		panic(fmt.Sprintf("messaging schema: could not compile embedded schema: %v", err))
	}
	return compiled
}

// Validate checks that a raw messaging event matches the ETOS SSEv2 messaging
// events schema. Events with an unknown type, or a data payload that is missing
// required fields or has an invalid value, are rejected.
func Validate(data []byte) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("event is not valid JSON: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return err
	}
	return nil
}
