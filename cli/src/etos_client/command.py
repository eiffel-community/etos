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
from etos_client.etos.v0.command import ETOSv0
from etos_client.etos.v1alpha.command import ETOSv1alpha

LOGGER = logging.getLogger(__name__)


class TestRun(Command):
    """ETOS client testrun.

    Usage: etosctl [-v|-vv] [options] testrun <command> [<args>...]

    Command is optional and will be set to the default version if not set.
    The default version can be seen using `etosctl testrun --version`

    Commands:
        v0            Version 0 of ETOS.
        v1alpha       Version v1alpha of ETOS.

    Options:
        -h,--help     Show this screen
        --version     Print version and exit
    """

    meta = CommandMeta(
        name="testrun",
        description="Operate on ETOS testruns",
        version="v0",
        subcommands={
            "v0": ETOSv0,
            "v1alpha": ETOSv1alpha,
        },
    )

    def parse_args(self, argv: list[str]) -> dict:
        """Parse arguments for etosctl testrun start."""
        # Check for an optional subcommand, if it does not exist set meta version as default.
        subcommand = False
        for cmd in self.meta.subcommands.keys():
            if cmd in argv:
                subcommand = True
        if not subcommand:
            argv = [self.meta.version] + argv
        return super().parse_args(argv)
