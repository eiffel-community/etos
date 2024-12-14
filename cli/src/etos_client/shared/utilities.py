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
"""ETOS shared utility functions."""
import os
from pathlib import Path


def directories(args: dict) -> tuple[Path, Path]:
    """Create directories for ETOS logs."""
    workspace = args["--workspace"] or os.getenv("WORKSPACE", os.getcwd())
    artifact_dir = args["--artifact-dir"] or "artifacts"
    report_dir = args["--report-dir"] or "reports"
    artifact_dir = Path(workspace).joinpath(artifact_dir)
    artifact_dir.mkdir(exist_ok=True)
    report_dir = Path(workspace).joinpath(report_dir)
    report_dir.mkdir(exist_ok=True)
    return report_dir, artifact_dir
