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
"""ETOS request data."""
import json
from uuid import UUID
from typing import Optional, Union

from pydantic import BaseModel, validator

# Pydantic requires validators first argument to be cls and the methods cannot be classmethods
# pylint:disable=no-self-argument


class RequestSchema(BaseModel):
    """Request model for the ETOS start API."""

    artifact_id: Optional[str]
    artifact_identity: Optional[str]
    test_suite_url: str
    dataset: Optional[Union[dict, list]] = {}
    execution_space_provider: Optional[str] = "default"
    iut_provider: Optional[str] = "default"
    log_area_provider: Optional[str] = "default"

    @classmethod
    def from_args(cls, args: dict) -> "RequestSchema":
        """Create a RequestSchema from an argument list from argparse."""
        return RequestSchema(
            artifact_id=args["--identity"],
            artifact_identity=args["--identity"],
            test_suite_url=args["--test-suite"],
            dataset=args["--dataset"],
            execution_space_provider=args["--execution-space-provider"] or "default",
            iut_provider=args["--iut-provider"] or "default",
            log_area_provider=args["--log-area-provider"] or "default",
        )

    @validator("artifact_identity", always=True)
    def trim_identity_if_necessary(cls, artifact_identity: Optional[str], values) -> Optional[str]:
        """Trim identity if id is set."""
        if values.get("artifact_id") is not None:
            return None
        return artifact_identity

    @validator("artifact_id", pre=True)
    def is_uuid(cls, artifact_id: Optional[str]) -> Optional[str]:
        """Test if string is a valid UUID v4."""
        try:
            UUID(artifact_id, version=4)
        except ValueError:
            return None
        return artifact_id

    @validator("dataset", always=True)
    def dataset_list_trimming(cls, dataset: Optional[Union[dict, list]]) -> list[dict]:
        """Trim away list completely should the dataset only contain a single index."""
        if dataset is None:
            return {}
        if len(dataset) == 1:
            return json.loads(dataset[0])
        return [json.loads(data) for data in dataset]


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
            event_repository=response.get("event_repository"),
            tercc=response.get("tercc"),
            artifact_id=response.get("artifact_id"),
            artifact_identity=response.get("artifact_identity"),
        )
