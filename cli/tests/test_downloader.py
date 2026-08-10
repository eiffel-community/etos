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
"""Tests for the ETOS client download reconciliation."""

import hashlib
from unittest.mock import MagicMock

from etos_client.shared.downloader import Downloadable, Downloader


class _FakeResponse:
    """Minimal stand-in for a streamed requests.Response."""

    def __init__(self, content: bytes):
        self.content = content

    def raise_for_status(self):
        """Pretend the HTTP request succeeded."""
        return None

    def __iter__(self):
        """Yield the body as a single chunk."""
        yield self.content


def _downloader(content: bytes) -> Downloader:
    """Create a Downloader whose HTTP client returns a fixed response."""
    downloader = Downloader()
    http = MagicMock()
    http.get.return_value = _FakeResponse(content)
    # Replace the name-mangled private Http client.
    setattr(downloader, "_Downloader__http", http)
    return downloader


def _download(downloader: Downloader, item: Downloadable) -> None:
    """Invoke the private download method synchronously."""
    downloader._Downloader__download(item)  # pylint:disable=protected-access


def test_successful_download_is_counted_and_not_missing(tmp_path):
    """A verified download is counted once and reported as not missing."""
    content = b"hello"
    item = Downloadable(url="http://example/a.txt", name="a.txt", path=tmp_path)
    downloader = _downloader(content)

    downloader.queue_download(item)
    _download(downloader, item)

    assert downloader.reconciler.expected == 1
    assert len(downloader.downloads) == 1
    assert downloader.reconciler.missing() == []
    assert downloader.failed is False
    assert (tmp_path / "a.txt").read_bytes() == content


def test_correct_checksum_passes(tmp_path):
    """A download whose checksum matches is counted."""
    content = b"hello world"
    digest = hashlib.sha256(content).hexdigest()
    item = Downloadable(
        url="http://example/b.txt",
        name="b.txt",
        path=tmp_path,
        checksums={"SHA-256": digest},
    )
    downloader = _downloader(content)

    downloader.queue_download(item)
    _download(downloader, item)

    assert len(downloader.downloads) == 1
    assert downloader.reconciler.missing() == []


def test_integrity_failure_is_missing_and_not_counted(tmp_path):
    """A checksum mismatch is not counted, is reported missing, and marks failure."""
    content = b"hello"
    item = Downloadable(
        url="http://example/a.txt",
        name="a.txt",
        path=tmp_path,
        checksums={"SHA-256": "deadbeef"},
    )
    downloader = _downloader(content)

    downloader.queue_download(item)
    _download(downloader, item)

    assert len(downloader.downloads) == 0
    assert downloader.failed is True
    missing = downloader.reconciler.missing()
    assert len(missing) == 1
    assert missing[0].url == item.url
    assert item.url in downloader.reconciler.failures
    # The corrupt file must not be left on disk.
    assert not (tmp_path / "a.txt").exists()


def test_queue_dedup_by_url(tmp_path):
    """Queuing the same URL twice only expects it once."""
    item = Downloadable(url="http://example/c.txt", name="c.txt", path=tmp_path)
    downloader = _downloader(b"x")

    downloader.queue_download(item)
    downloader.queue_download(item)

    assert downloader.reconciler.expected == 1


def test_breakdown_groups_by_sub_suite(tmp_path):
    """The breakdown groups downloaded/expected file counts per sub-suite."""
    suite_0 = tmp_path / "SubSuite_0"
    suite_1 = tmp_path / "SubSuite_1"
    item_a = Downloadable(url="http://example/a.txt", name="a.txt", path=suite_0)
    item_b = Downloadable(url="http://example/b.txt", name="b.txt", path=suite_0)
    item_c = Downloadable(url="http://example/c.txt", name="c.txt", path=suite_1)
    downloader = _downloader(b"data")

    for item in (item_a, item_b, item_c):
        downloader.queue_download(item)
        _download(downloader, item)

    breakdown = downloader.reconciler.breakdown()
    assert breakdown[str(suite_0)] == (2, 2)
    assert breakdown[str(suite_1)] == (1, 1)
