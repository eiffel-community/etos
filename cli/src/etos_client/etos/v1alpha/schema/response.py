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
"""ETOS v1 response schema."""
from uuid import UUID
from pydantic import BaseModel


class ResponseSchema(BaseModel):
    """Response model for the ETOS start API."""

    event_repository: str
    tercc: UUID
    artifact_id: UUID
    artifact_identity: str

    @classmethod
    def from_response(cls, response: dict) -> "ResponseSchema":
        """Create a ResponseSchema from a dictionary. Typically used for ETOS API http response."""
        return ResponseSchema(
            event_repository=response.get("event_repository"),  # type: ignore
            tercc=response.get("tercc"),  # type: ignore
            artifact_id=response.get("artifact_id"),  # type: ignore
            artifact_identity=response.get("artifact_identity"),  # type: ignore
        )
