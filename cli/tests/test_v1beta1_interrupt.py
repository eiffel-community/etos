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
"""Tests for interrupting a v1beta1 testrun."""

import signal
from functools import partial
from pathlib import Path
from unittest.mock import Mock
from uuid import UUID

import pytest
from requests.exceptions import HTTPError

from etos_client.etos.v1beta1 import etos as etos_module
from etos_client.etos.v1beta1.etos import Etos
from etos_client.etos.v1beta1.schema.response import ResponseSchema


def _send_signal(signum: signal.Signals) -> None:
    """Invoke the signal handler currently registered for a signal."""
    handler = signal.getsignal(signum)
    assert callable(handler)
    handler(signum, None)


def _interrupt_track(signum: signal.Signals, *_args: object) -> None:
    """Simulate an interrupt while the testrun is being tracked."""
    _send_signal(signum)


def _abort_with_signals_ignored(abort_response: Mock, *_args: object, **_kwargs: object) -> Mock:
    """Return the response after checking repeated signals are ignored."""
    assert signal.getsignal(signal.SIGINT) == signal.SIG_IGN
    assert signal.getsignal(signal.SIGTERM) == signal.SIG_IGN
    return abort_response


@pytest.mark.parametrize(
    ("signum", "expected_status"),
    [(signal.SIGINT, 130), (signal.SIGTERM, 143)],
)
@pytest.mark.parametrize("abort_error", [None, HTTPError("404 Client Error")])
def test_interrupt_aborts_testrun_and_preserves_signal_status(
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
    signum: signal.Signals,
    expected_status: int,
    abort_error: HTTPError | None,
) -> None:
    """Abort the active testrun and return a signal status, including on API errors."""
    etos = Etos({"<cluster>": "http://localhost", "-v": False}, Mock())
    response = ResponseSchema(
        event_repository="http://localhost/events",
        tercc=UUID("12345678-1234-4321-abcd-123456789abc"),
        artifact_id=UUID("12345678-1234-4321-abcd-123456789abd"),
        artifact_identity="test-artifact",
    )
    mocks = Mock()
    mocks.downloader.downloads = []
    mocks.downloader.failed = False
    mocks.downloader.reconciler.missing.return_value = False
    if abort_error is not None:
        mocks.abort_response.raise_for_status.side_effect = abort_error
    mocks.http.delete.side_effect = partial(_abort_with_signals_ignored, mocks.abort_response)
    http_factory = Mock(return_value=mocks.http)
    original_handlers = {
        signal.SIGINT: signal.getsignal(signal.SIGINT),
        signal.SIGTERM: signal.getsignal(signal.SIGTERM),
    }

    monkeypatch.setattr(etos_module, "Downloader", lambda: mocks.downloader)
    monkeypatch.setattr(etos_module, "V1Beta1TestRun", lambda *_args: mocks.test_run)
    monkeypatch.setattr(etos_module, "Http", http_factory)
    monkeypatch.setattr(
        etos_module,
        "directories",
        lambda _args: (Path.cwd() / "reports", Path.cwd() / "artifacts"),
    )
    monkeypatch.setattr(etos_module.shutil, "make_archive", Mock())
    monkeypatch.setattr(
        etos,
        "_Etos__track",  # pylint: disable=protected-access
        partial(_interrupt_track, signum),
    )

    with pytest.raises(SystemExit) as exit_info:
        etos._Etos__wait(response)  # pylint: disable=protected-access

    assert exit_info.value.code == expected_status
    http_factory.assert_called_once_with(
        retry=etos_module.HTTP_RETRY_PARAMETERS,
        timeout=etos_module.START_REQUEST_TIMEOUT,
    )
    mocks.http.delete.assert_called_once_with(
        f"http://localhost/api/v1beta1/testrun/{response.tercc}",
        headers=etos.headers,
    )
    mocks.abort_response.raise_for_status.assert_called_once_with()
    mocks.downloader.stop.assert_called_once_with(False)
    mocks.downloader.join.assert_called_once_with()
    assert signal.getsignal(signal.SIGINT) == original_handlers[signal.SIGINT]
    assert signal.getsignal(signal.SIGTERM) == original_handlers[signal.SIGTERM]
    if abort_error is not None:
        assert "Failed to abort ETOS testrun" in caplog.text
