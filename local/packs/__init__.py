# Copyright Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# 	http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Local ETOS packs."""

from .cert_manager import CertManager
from .controller import Controller
from .etos import Etos
from .prometheus import Prometheus  # noqa: F401
from .verify import Verify

# Prometheus is no longer a default pack: the controller exports OTLP and is not scraped.
# The Prometheus pack remains importable for optional, explicit use.
PACKS = [CertManager, Controller, Etos, Verify]
