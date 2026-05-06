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
"""Tests for the v1alpha RequestSchema timeout/deadline support."""

import pytest

from etos_client.etos.v1alpha.schema.request import RequestSchema


def _make_args(**overrides):
    """Create a minimal args dict mimicking docopt output, allowing overrides."""
    defaults = {
        "--identity": "12345678-1234-4321-abcd-123456789abc",
        "--parent-activity": None,
        "--test-suite": "http://example.com/suite.yaml",
        "--dataset": [],
        "--execution-space-provider": "default",
        "--iut-provider": "default",
        "--log-area-provider": "default",
        "--timeout": None,
        "--deadline": None,
    }
    defaults.update(overrides)
    return defaults


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


class TestTimeoutDeadlineFlags:
    """Test that timeout and deadline CLI flags work correctly."""

    def test_timeout_from_args(self):
        """Test that --timeout flag is passed through from_args."""
        args = _make_args(**{"--timeout": "3600"})
        request = RequestSchema.from_args(args)
        assert request.timeout == 3600
        assert request.deadline is None

    def test_deadline_from_args(self):
        """Test that --deadline flag is passed through from_args."""
        args = _make_args(**{"--deadline": "1749200000"})
        request = RequestSchema.from_args(args)
        assert request.deadline == 1749200000
        assert request.timeout is None

    def test_timeout_and_deadline_raises(self):
        """Test that setting both timeout and deadline raises an error."""
        with pytest.raises(ValueError, match="Only one of 'timeout' or 'deadline'"):
            _make_request(timeout=3600, deadline=1749200000)

    def test_neither_timeout_nor_deadline(self):
        """Test that neither is set when not provided."""
        args = _make_args()
        request = RequestSchema.from_args(args)
        assert request.timeout is None
        assert request.deadline is None

    def test_timeout_included_in_model_dump(self):
        """Test that timeout appears in model_dump for API requests."""
        request = _make_request(timeout=1800)
        dumped = request.model_dump()
        assert dumped["timeout"] == 1800
        assert dumped["deadline"] is None

    def test_deadline_included_in_model_dump(self):
        """Test that deadline appears in model_dump for API requests."""
        request = _make_request(deadline=1749200000)
        dumped = request.model_dump()
        assert dumped["deadline"] == 1749200000
        assert dumped["timeout"] is None

    def test_none_args_produce_none_fields(self):
        """Test that None args (docopt default) result in None fields."""
        args = _make_args(**{"--timeout": None, "--deadline": None})
        request = RequestSchema.from_args(args)
        assert request.timeout is None
        assert request.deadline is None

    def test_false_args_produce_none_fields(self):
        """Test that False args (docopt when flag not provided) result in None fields."""
        args = _make_args(**{"--timeout": False, "--deadline": False})
        request = RequestSchema.from_args(args)
        assert request.timeout is None
        assert request.deadline is None
