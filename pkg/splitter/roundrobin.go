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
package splitter

import (
	"github.com/eiffel-community/etos/api/v1alpha1"
)

// RoundRobin implements the Splitter interface with a round-robin strategy
type RoundRobin struct {
	suites  [][]v1alpha1.Test
	current uint64
}

// NewRoundRobinSplitter creates a new round robin splitter.
func NewRoundRobinSplitter() Splitter {
	return &RoundRobin{}
}

// SetSize sets the size of the test slice.
func (s *RoundRobin) SetSize(size int) Splitter {
	s.suites = make([][]v1alpha1.Test, size)
	return s
}

// nextItem gets the next item in the round robin test slice.
func (s *RoundRobin) nextIndex() int {
	nextIndex := int(s.current) % len(s.suites)
	s.current++
	return nextIndex
}

// AddTest to the splitter.
func (s *RoundRobin) AddTest(test v1alpha1.Test) {
	index := s.nextIndex()
	s.suites[index] = append(s.suites[index], test)
}

// Split the tests. For the round robin splitter they are already split at this point.
func (s *RoundRobin) Split() [][]v1alpha1.Test {
	return s.suites
}
