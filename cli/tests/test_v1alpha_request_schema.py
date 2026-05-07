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
"""Tests for the v1alpha RequestSchema timeout support."""

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


class TestTimeoutFlag:
    """Test that timeout CLI flag works correctly."""

    def test_timeout_from_args(self):
        """Test that --timeout flag is passed through from_args."""
        args = _make_args(**{"--timeout": "3600"})
        request = RequestSchema.from_args(args)
        assert request.timeout == 3600

    def test_timeout_not_set(self):
        """Test that timeout is None when not provided."""
        args = _make_args()
        request = RequestSchema.from_args(args)
        assert request.timeout is None

    def test_timeout_included_in_model_dump(self):
        """Test that timeout appears in model_dump for API requests."""
        request = _make_request(timeout=1800)
        dumped = request.model_dump()
        assert dumped["timeout"] == 1800

    def test_timeout_none_in_model_dump(self):
        """Test that timeout is None in model_dump when not set."""
        request = _make_request()
        dumped = request.model_dump()
        assert dumped["timeout"] is None

    def test_timeout_string_converted_to_int(self):
        """Test that pydantic converts a string timeout to int."""
        args = _make_args(**{"--timeout": "7200"})
        request = RequestSchema.from_args(args)
        assert request.timeout == 7200
        assert isinstance(request.timeout, int)
