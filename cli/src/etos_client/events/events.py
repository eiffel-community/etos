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
"""ETOS client event schemas."""
from typing import Optional
from pydantic import BaseModel


class Activity(BaseModel):
    """ETOS activity events."""

    triggered: Optional[dict]
    canceled: Optional[dict]
    finished: Optional[dict]


class Environment(BaseModel):
    """ETOS environment events."""

    name: str
    uri: Optional[str]


class Announcement(BaseModel):
    """ETOS announcements."""

    heading: str
    body: str


class Artifact(BaseModel):
    """ETOS artifact."""

    files: list[str]
    suite_name: str
    location: str


class SubSuite(BaseModel):
    """ETOS sub suite events."""

    started: Optional[dict]
    finished: Optional[dict]


class TestSuite(BaseModel):
    """ETOS main suite events."""

    started: Optional[dict]
    finished: Optional[dict]
    sub_suites: list[SubSuite] = []


class Events(BaseModel):
    """ETOS events."""

    activity: Activity = Activity()
    tercc: Optional[dict]
    main_suites: list[TestSuite] = []
    environments: list[Environment] = []
    announcements: list[Announcement] = []
    artifacts: list[Artifact] = []
