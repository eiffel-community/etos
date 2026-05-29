// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package suite_v1alpha1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// schemaDir returns the absolute path to the directory containing this test file.
func schemaDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

// TestExampleValidatesAgainstSchema loads the JSON schema and validates
// the example YAML file against it. This ensures the example stays in
// sync with the schema definition.
func TestExampleValidatesAgainstSchema(t *testing.T) {
	dir := schemaDir()

	// Load and compile schema.
	schemaPath := filepath.Join(dir, "suite.schema.json")
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatalf("failed to compile schema %s: %v", schemaPath, err)
	}

	// Load example YAML and convert to JSON-compatible types.
	examplePath := filepath.Join(dir, "example.yaml")
	yamlBytes, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("failed to read example %s: %v", examplePath, err)
	}
	var yamlData interface{}
	if err := yaml.Unmarshal(yamlBytes, &yamlData); err != nil {
		t.Fatalf("failed to parse example YAML: %v", err)
	}
	jsonData := toJSONCompatible(yamlData)

	// Validate.
	if err := schema.Validate(jsonData); err != nil {
		t.Errorf("example.yaml does not validate against suite.schema.json:\n%v", err)
	}
}

// TestSchemaRejectsInvalidDocument ensures the schema correctly rejects
// a document that violates required constraints.
func TestSchemaRejectsInvalidDocument(t *testing.T) {
	dir := schemaDir()

	schemaPath := filepath.Join(dir, "suite.schema.json")
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatalf("failed to compile schema %s: %v", schemaPath, err)
	}

	tests := []struct {
		name string
		doc  string
	}{
		{
			name: "empty object",
			doc:  `{}`,
		},
		{
			name: "missing test_executions",
			doc:  `{"suites": [{"priority": 1}]}`,
		},
		{
			name: "missing required execution fields",
			doc: `{"suites": [{"priority": 1, "test_executions": [
{"id": "t1", "testcase": {"id": "tc1"},
 "execution": {},
 "environment": {"test_runner": "img"}}
]}]}`,
		},
		{
			name: "extra property not allowed",
			doc: `{"suites": [{"priority": 1, "test_executions": [
{"id": "t1", "testcase": {"id": "tc1"},
 "execution": {"command": "exit 0"},
 "environment": {"test_runner": "img"},
 "unknown_field": true}
]}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data interface{}
			if err := json.Unmarshal([]byte(tt.doc), &data); err != nil {
				t.Fatalf("invalid test JSON: %v", err)
			}
			if err := schema.Validate(data); err == nil {
				t.Errorf("expected validation to fail for %q, but it passed", tt.name)
			}
		})
	}
}

// toJSONCompatible recursively converts yaml.v3 types to JSON-compatible
// types that the jsonschema library expects (map[string]interface{} and
// []interface{} instead of map[interface{}]interface{}).
func toJSONCompatible(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			out[k] = toJSONCompatible(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v := range val {
			out[i] = toJSONCompatible(v)
		}
		return out
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return val
	}
}
