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

package v1alpha2

import (
	"context"
	"encoding/json"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fakeReader implements the client.Reader interface
type fakeReader struct {
	getResponse  client.Object
	listResponse client.ObjectList
}

// FakeReader returns a fake client implementing the client.Reader interface.
//
// Any of the two inputs can be nil, but it is best to at least provide one of them.
// If an input is nil, the corresponding method will return an error.
// The inputs are the resources you expect a webhook to fetch either using .Get or
// .List.
func FakeReader(get client.Object, list client.ObjectList) client.Reader {
	return &fakeReader{get, list}
}

// Get fakes the get response of a controller-runtime client.
func (f *fakeReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getResponse == nil {
		return errors.New("this is an error in the Get method")
	}
	b, err := json.Marshal(f.getResponse)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, obj)
}

// List fakes the list response of a controller-runtime client.
func (f *fakeReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.listResponse == nil {
		return errors.New("this is an error in the List method")
	}
	b, err := json.Marshal(f.listResponse)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, list)
}
