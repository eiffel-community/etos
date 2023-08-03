# -*- coding: utf-8 -*-
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
"""ETOS client testrun."""
import logging

from etosctl.command import Command
from etosctl.models import CommandMeta
from etos_client.start import Start

LOGGER = logging.getLogger(__name__)


class TestRun(Command):
    """ETOS client testrun.

    Usage: etosctl [-v|-vv] [options] testrun <command> [<args>...]

    Commands:
        start         Start a new ETOS testrun

    Options:
        -h,--help     Show this screen
        --version     Print version and exit
    """

    meta = CommandMeta(
        name="testrun",
        description="Operate on ETOS testruns",
        version="v1alpha1",
        subcommands={"start": Start},
    )
