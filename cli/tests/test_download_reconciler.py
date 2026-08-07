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
"""Tests for the standalone download reconciler."""

from pathlib import Path

from etos_client.shared.downloader import Downloadable
from etos_client.shared.download_reconciler import DownloadReconciler


def _item(url: str, name: str, directory: str) -> Downloadable:
    """Build a Downloadable pointing at a sub-suite directory."""
    return Downloadable(url=url, name=name, path=Path(directory))


def test_received_and_downloaded_is_not_missing():
    """An artifact recorded as downloaded is not reported missing."""
    reconciliation = DownloadReconciler()
    item = _item("http://example/a.txt", "a.txt", "SubSuite_0")

    reconciliation.record_received(item)
    reconciliation.record_downloaded(item)

    assert reconciliation.expected == 1
    assert reconciliation.missing() == []


def test_received_but_failed_is_missing():
    """An artifact that failed to download is reported missing with its reason."""
    reconciliation = DownloadReconciler()
    item = _item("http://example/a.txt", "a.txt", "SubSuite_0")

    reconciliation.record_received(item)
    reconciliation.record_failed(item, "boom")

    missing = reconciliation.missing()
    assert [entry.url for entry in missing] == [item.url]
    assert reconciliation.failures[item.url] == "boom"


def test_breakdown_groups_by_directory():
    """Counts are grouped per destination directory (sub-suite)."""
    reconciliation = DownloadReconciler()
    item_a = _item("http://example/a.txt", "a.txt", "SubSuite_0")
    item_b = _item("http://example/b.txt", "b.txt", "SubSuite_0")
    item_c = _item("http://example/c.txt", "c.txt", "SubSuite_1")

    for item in (item_a, item_b, item_c):
        reconciliation.record_received(item)
    reconciliation.record_downloaded(item_a)

    breakdown = reconciliation.breakdown()
    assert breakdown["SubSuite_0"] == (1, 2)
    assert breakdown["SubSuite_1"] == (0, 1)


def test_received_dedups_by_url():
    """Recording the same URL twice only expects it once."""
    reconciliation = DownloadReconciler()
    item = _item("http://example/a.txt", "a.txt", "SubSuite_0")

    reconciliation.record_received(item)
    reconciliation.record_received(item)

    assert reconciliation.expected == 1
