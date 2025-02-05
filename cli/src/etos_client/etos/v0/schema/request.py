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
"""ETOS v0 schema."""
import json
from uuid import UUID

from typing import Optional, Union
from pydantic import BaseModel, ValidationInfo, field_validator


class RequestSchema(BaseModel):
    """Schema for ETOSv0 API requests."""

    artifact_id: Optional[str]
    artifact_identity: Optional[str]
    parent_activity: Optional[str]
    test_suite_url: str
    dataset: Optional[Union[dict, list]] = {}
    execution_space_provider: Optional[str] = "default"
    iut_provider: Optional[str] = "default"
    log_area_provider: Optional[str] = "default"

    @classmethod
    def from_args(cls, args: dict) -> "RequestSchema":
        """Create a RequestSchema from an argument list from argparse."""
        parent_activity = args["--parent-activity"] if "--parent-activity" in args else None
        execution_space_provider = args["--execution-space-provider"] if "--execution-space-provider" in args else "default"
        iut_provider = args["--iut-provider"] if "--iut-provider" in args else "default"
        log_area_provider = args["--log-area-provider"] if "--log-area-provider" in args else "default"

        return RequestSchema(
            artifact_id=args["--identity"],
            artifact_identity=args["--identity"],
            parent_activity=parent_activity,
            test_suite_url=args["--test-suite"],
            dataset=args["--dataset"],
            execution_space_provider=execution_space_provider,
            iut_provider=iut_provider,
            log_area_provider=log_area_provider,
        )

    @field_validator("artifact_identity")
    def trim_identity_if_necessary(
        cls, artifact_identity: Optional[str], info: ValidationInfo
    ) -> Optional[str]:
        """Trim identity if id is set."""
        if info.data.get("artifact_id") is not None:
            return None
        return artifact_identity

    @field_validator("artifact_id")
    def is_uuid(cls, artifact_id: Optional[str]) -> Optional[str]:
        """Test if string is a valid UUID v4."""
        try:
            UUID(artifact_id, version=4)
        except ValueError:
            return None
        return artifact_id

    @field_validator("dataset")
    def dataset_list_trimming(cls, dataset: Optional[Union[dict, list]]) -> list[dict]:
        """Trim away list completely should the dataset only contain a single index."""
        if dataset is None:
            return [{}]
        if len(dataset) == 1:
            return json.loads(dataset[0])
        return [json.loads(data) for data in dataset]
