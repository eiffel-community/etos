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
"""Reconciliation of artifacts received from test runners against what was downloaded."""

import logging
from threading import Lock
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from etos_client.shared.downloader import Downloadable


class DownloadReconciler:
    """Reconcile artifacts received by the client against those actually downloaded.

    The downloader reports three events to this class: an artifact was received for
    download, an artifact was successfully downloaded and verified, or an artifact
    failed to download. From those events the reconciler determines which received
    artifacts never arrived intact.

    The scope is limited to artifacts the client was told about: it verifies that every
    artifact received (as a report/artifact event) was successfully downloaded and
    integrity-verified. It does not verify that the test runners published an event for
    every file they produced, nor does it compare against the raw upload/download counts.
    """

    logger = logging.getLogger(__name__)

    def __init__(self) -> None:
        """Init."""
        self.__lock = Lock()
        self.__received: dict[str, "Downloadable"] = {}
        self.__downloaded: set[str] = set()
        self.failures: dict[str, str] = {}

    def record_received(self, item: "Downloadable") -> None:
        """Record that an artifact was received from a test runner for download."""
        with self.__lock:
            self.__received.setdefault(item.url, item)

    def record_downloaded(self, item: "Downloadable") -> None:
        """Record that an artifact was successfully downloaded and verified."""
        with self.__lock:
            self.__downloaded.add(item.url)

    def record_failed(self, item: "Downloadable", reason: str) -> None:
        """Record that an artifact failed to download, with the reason why."""
        with self.__lock:
            self.failures[item.url] = reason

    @property
    def expected(self) -> int:
        """Number of unique artifacts received from the test runners for download."""
        with self.__lock:
            return len(self.__received)

    def missing(self) -> list["Downloadable"]:
        """Return artifacts that were received but not successfully downloaded.

        An artifact is considered missing if it was received for download but never
        completed a successful download and integrity verification.
        """
        with self.__lock:
            downloaded = set(self.__downloaded)
            received = dict(self.__received)
        return [item for url, item in received.items() if url not in downloaded]

    def breakdown(self) -> dict[str, tuple[int, int]]:
        """Return per-destination (downloaded, expected) file counts.

        Files are grouped by their destination directory. A sub-suite directory holds
        that sub-suite's artifacts, while the reports directory holds logs shared across
        all sub-suites. Returns a mapping of directory to a (downloaded, expected) tuple.
        """
        with self.__lock:
            downloaded = set(self.__downloaded)
            received = dict(self.__received)
        counts: dict[str, tuple[int, int]] = {}
        for url, item in received.items():
            key = str(item.path)
            done, expected = counts.get(key, (0, 0))
            counts[key] = (done + (1 if url in downloaded else 0), expected + 1)
        return counts

    def log_summary(self) -> None:
        """Log a reconciliation summary of received vs. downloaded artifacts."""
        missing = self.missing()
        if missing:
            self.logger.error(
                "Artifact reconciliation FAILED: %d of %d received artifacts were not "
                "downloaded successfully.",
                len(missing),
                self.expected,
            )
            for item in missing:
                self.logger.error(
                    "Missing artifact %r from %s (%s)",
                    item.name,
                    item.url,
                    self.failures.get(item.url, "not downloaded"),
                )
        else:
            self.logger.info(
                "Artifact reconciliation OK: all %d received artifacts were downloaded "
                "successfully.",
                self.expected,
            )
        breakdown = self.breakdown()
        for destination, (downloaded, expected) in sorted(breakdown.items()):
            self.logger.info("  %s: %d/%d files downloaded", destination, downloaded, expected)
