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
"""Command line for managing ETOSv1alpha testruns."""
from etosctl.command import SubCommand
from etosctl.models import CommandMeta

from .subcommands.start import Start


class ETOSv1alpha(SubCommand):
    """
    Client for managing testruns in ETOS.

    Usage: etosctl testrun [-v|-vv] [options] v1alpha <command> [<args>...]

    Commands:
        start         Start a new ETOS testrun via the v1alpha API

    Options:
        -h, --help    Show this help message and exit
        --version     Show program's version number and exit
    """

    meta: CommandMeta = CommandMeta(
        name="v1alpha",
        description="Manage ETOSv1alpha testruns.",
        version="v1alpha",
        subcommands={"start": Start},
    )
