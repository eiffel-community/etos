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
"""ETOS v1 request schema."""

import json
from typing import Optional, Union
from uuid import UUID

from pydantic import BaseModel, ValidationInfo, field_validator, model_validator


def _parse_optional_int(value: object) -> Optional[int]:
    """Parse a value to an optional int, returning None for falsy values."""
    if value is None or value is False:
        return None
    return int(value)


class RequestSchema(BaseModel):
    """Schema for ETOSv1 API requests."""

    artifact_id: Optional[str]
    artifact_identity: Optional[str]
    parent_activity: Optional[str]
    test_suite_url: str
    dataset: Optional[Union[dict, list]] = {}
    execution_space_provider: Optional[str] = "default"
    iut_provider: Optional[str] = "default"
    log_area_provider: Optional[str] = "default"
    timeout: Optional[int] = None
    deadline: Optional[int] = None

    @classmethod
    def from_args(cls, args: dict) -> "RequestSchema":
        """Create a RequestSchema from an argument list from argparse."""
        return RequestSchema(
            artifact_id=args["--identity"],
            artifact_identity=args["--identity"],
            parent_activity=args["--parent-activity"] or None,
            test_suite_url=args["--test-suite"],
            dataset=args["--dataset"],
            execution_space_provider=args["--execution-space-provider"] or "default",
            iut_provider=args["--iut-provider"] or "default",
            log_area_provider=args["--log-area-provider"] or "default",
            timeout=_parse_optional_int(args.get("--timeout")),
            deadline=_parse_optional_int(args.get("--deadline")),
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

    @model_validator(mode="after")
    def validate_timeout_or_deadline(self) -> "RequestSchema":
        """Validate that only one of timeout or deadline is set.

        The controller will ignore timeout if deadline is set, so we should
        prevent users from setting both to avoid confusion.

        :return: The validated model.
        :rtype: RequestSchema
        """
        if self.timeout is not None and self.deadline is not None:
            raise ValueError("Only one of 'timeout' or 'deadline' can be set, not both.")
        return self
