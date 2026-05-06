# Copyright Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Tests for the v1alpha RequestSchema, specifically timeout/deadline extraction."""

import json

import pytest

from etos_client.etos.v1alpha.schema.request import RequestSchema


def _make_request(**kwargs):
    """Create a RequestSchema with sensible defaults, allowing overrides."""
    defaults = {
        "artifact_id": "12345678-1234-4321-abcd-123456789abc",
        "artifact_identity": "pkg:test/identity",
        "parent_activity": None,
        "test_suite_url": "http://example.com/suite.yaml",
        "dataset": None,
        "execution_space_provider": "default",
        "iut_provider": "default",
        "log_area_provider": "default",
    }
    defaults.update(kwargs)
    return RequestSchema(**defaults)


class TestTimeoutDeadlineExtraction:
    """Test that timeout and deadline are extracted from the dataset."""

    def test_timeout_extracted_from_dataset(self):
        """Test that timeout is extracted from dataset and set on the model."""
        request = _make_request(dataset=[json.dumps({"timeout": 3600})])
        assert request.timeout == 3600
        assert request.deadline is None
        # timeout should be removed from the dataset dict
        ds = request.dataset if isinstance(request.dataset, list) else [request.dataset]
        for data in ds:
            assert "timeout" not in data

    def test_deadline_extracted_from_dataset(self):
        """Test that deadline is extracted from dataset and set on the model."""
        request = _make_request(dataset=[json.dumps({"deadline": 1749200000})])
        assert request.deadline == 1749200000
        assert request.timeout is None
        ds = request.dataset if isinstance(request.dataset, list) else [request.dataset]
        for data in ds:
            assert "deadline" not in data

    def test_timeout_and_deadline_raises(self):
        """Test that setting both timeout and deadline raises an error."""
        with pytest.raises(ValueError, match="Only one of 'timeout' or 'deadline'"):
            _make_request(dataset=[json.dumps({"timeout": 3600, "deadline": 1749200000})])

    def test_no_timeout_or_deadline(self):
        """Test that neither timeout nor deadline is set when not in dataset."""
        request = _make_request(dataset=[json.dumps({"some_key": "some_value"})])
        assert request.timeout is None
        assert request.deadline is None

    def test_timeout_with_other_dataset_keys(self):
        """Test that other dataset keys are preserved when timeout is extracted."""
        request = _make_request(
            dataset=[json.dumps({"timeout": 7200, "pool": "mypool", "count": 2})]
        )
        assert request.timeout == 7200
        # Single dataset element is stored as a dict after validation
        ds = request.dataset if isinstance(request.dataset, dict) else request.dataset[0]
        assert ds == {"pool": "mypool", "count": 2}

    def test_empty_dataset(self):
        """Test that empty dataset works with no timeout/deadline."""
        request = _make_request(dataset=None)
        assert request.timeout is None
        assert request.deadline is None
        assert request.dataset == [{}]

    def test_timeout_included_in_model_dump(self):
        """Test that timeout appears in model_dump for API requests."""
        request = _make_request(dataset=[json.dumps({"timeout": 1800})])
        dumped = request.model_dump()
        assert dumped["timeout"] == 1800
        assert dumped["deadline"] is None

    def test_deadline_included_in_model_dump(self):
        """Test that deadline appears in model_dump for API requests."""
        request = _make_request(dataset=[json.dumps({"deadline": 1749200000})])
        dumped = request.model_dump()
        assert dumped["deadline"] == 1749200000
        assert dumped["timeout"] is None

    def test_timeout_as_string_is_cast_to_int(self):
        """Test that a string timeout value is cast to int."""
        request = _make_request(dataset=[json.dumps({"timeout": "3600"})])
        assert request.timeout == 3600
        assert isinstance(request.timeout, int)

    def test_multiple_datasets_timeout_from_last(self):
        """Test that timeout from the last dataset dict wins if present in multiple."""
        request = _make_request(
            dataset=[
                json.dumps({"timeout": 100, "pool": "a"}),
                json.dumps({"timeout": 200, "pool": "b"}),
            ]
        )
        assert request.timeout == 200
        for data in request.dataset:
            assert "timeout" not in data
