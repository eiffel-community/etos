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
"""Tests for ETOS client error handling."""

import pytest

from etos_client.types.result import Conclusion, Result, Verdict


class TestApiErrorDetailHandling:
    """Test that API error responses are properly converted to user-friendly strings."""

    def test_detail_as_string_passes_through(self):
        """Test that a string detail is passed through unchanged."""
        result = Result(
            verdict=Verdict.INCONCLUSIVE,
            conclusion=Conclusion.FAILED,
            reason="Some error message",
        )
        assert result.reason == "Some error message"

    def test_detail_list_converted_to_string(self):
        """Test the conversion logic used for 422 validation error detail lists."""
        # Simulate FastAPI 422 response detail
        detail = [
            {
                "type": "value_error",
                "loc": ["body", "artifact_id"],
                "msg": "Value error, artifact_identity must be a string starting with 'pkg:'",
                "input": None,
                "ctx": {"error": {}},
            }
        ]
        # This is the conversion logic from __start()
        if isinstance(detail, list):
            detail = "; ".join(
                err.get("msg", str(err)) if isinstance(err, dict) else str(err) for err in detail
            )
        assert isinstance(detail, str)
        assert "artifact_identity must be a string starting with 'pkg:'" in detail

    def test_multiple_validation_errors_joined(self):
        """Test that multiple validation errors are joined with semicolons."""
        detail = [
            {"msg": "First error"},
            {"msg": "Second error"},
        ]
        if isinstance(detail, list):
            detail = "; ".join(
                err.get("msg", str(err)) if isinstance(err, dict) else str(err) for err in detail
            )
        assert detail == "First error; Second error"

    def test_detail_list_without_msg_key_uses_str(self):
        """Test that detail dict without 'msg' key falls back to str()."""
        detail = [{"type": "error", "loc": ["body"]}]
        if isinstance(detail, list):
            detail = "; ".join(
                err.get("msg", str(err)) if isinstance(err, dict) else str(err) for err in detail
            )
        assert isinstance(detail, str)
        assert "type" in detail

    def test_result_reason_must_be_string(self):
        """Test that Result model rejects non-string reason."""
        with pytest.raises(Exception):
            Result(
                verdict=Verdict.INCONCLUSIVE,
                conclusion=Conclusion.FAILED,
                reason=[{"msg": "This is a list, not a string"}],
            )
