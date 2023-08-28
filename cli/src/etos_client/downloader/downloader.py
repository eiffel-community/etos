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
"""ETOS Client log downloader module."""
import logging
import time
import traceback
from pathlib import Path
from typing import Union, Optional
from threading import Thread, Lock
from multiprocessing.pool import ThreadPool
from queue import Queue, Empty
from json import JSONDecodeError

from pydantic import BaseModel
from urllib3.exceptions import MaxRetryError, NewConnectionError
import requests
from requests.exceptions import HTTPError

from etos_client.events.events import Artifact, TestSuite, SubSuite


class Downloadable(BaseModel):
    """Represent a downloadable file."""

    uri: str
    name: Optional[Path]


class Downloader(Thread):  # pylint:disable=too-many-instance-attributes
    """Log downloader for ETOS client."""

    logger = logging.getLogger(__name__)
    started = False

    def __init__(self, report_dir: Path, artifact_dir: Path) -> None:
        """Init."""
        super().__init__()
        self.__download_queue = Queue()
        self.__queued = []
        self.__exit = False
        self.__clear_queue = True
        self.__lock = Lock()
        self.failed: bool = False
        self.downloads: {str} = set()

        self.__report_dir = report_dir
        self.__artifact_dir = artifact_dir

    def __retry_download(self, item: Downloadable) -> None:
        """Download files."""
        self.logger.debug("Downloading %r", item)
        end_time = time.time() + 60
        while time.time() < end_time:
            response = requests.get(item.uri, stream=True, timeout=10)
            self.logger.debug("Download response: %r", response)
            if self.__should_retry(response):
                self.logger.debug("Download of %r failed. Retrying..", item)
                time.sleep(10)
                continue
            if not response.ok:
                with self.__lock:
                    self.failed = True
                self.logger.critical("Failed to download %r", item)
                return
            self.__save_file(item, response)
            return
        with self.__lock:
            self.failed = True
        self.logger.critical("Failed to download %r", item)
        return

    def __save_file(self, item: Downloadable, response: requests.Response) -> None:
        """Save downloaded file data to disk."""
        download_name = item.name
        index = 0
        while download_name.exists():
            index += 1
            download_name = download_name.with_name(f"{index}_{item.name.name}")
        self.logger.debug("Saving file %s", download_name)
        with self.__lock:
            self.downloads.add(download_name)
        with open(download_name, "wb+") as report:
            for chunk in response:
                report.write(chunk)

    def __should_retry(self, response: requests.Response) -> bool:
        """Check response to see whether it is worth retrying or not."""
        try:
            response.raise_for_status()
        except HTTPError as http_error:
            if http_error.response.status_code == 404:
                self.logger.warning("File not found")
                # Retry as it may just be that the file is being uploaded.
                return True
            if 400 <= http_error.response.status_code < 500:
                try:
                    response_json = response.json()
                except JSONDecodeError:
                    self.logger.debug("Raw response from download: %r", response.text)
                    response_json = {
                        "detail": "Unknown client error when downloading files from log area"
                    }
                self.logger.critical("Download response: %r", response_json)
                return False
            return True
        except (
            ConnectionError,
            NewConnectionError,
            MaxRetryError,
            TimeoutError,
        ):
            self.logger.exception("Network connectivity error when downloading logs and artifacts.")
            return True
        return False

    def __trigger_download(self, pool: ThreadPool) -> bool:
        """Get downloadable from queue and add it to threadpool."""
        try:
            item = self.__download_queue.get_nowait()
            pool.apply_async(
                self.__retry_download,
                error_callback=self.__print_traceback,
                args=(item,),
            )
            return True
        except Empty:
            return False

    def __print_traceback(self, value: Exception) -> None:
        """Print a traceback if error in pool."""
        try:
            raise value
        except:  # pylint:disable=bare-except
            with self.__lock:
                self.failed = True
            traceback.print_exc()

    def run(self) -> None:
        """Run the log downloader thread."""
        self.started = True
        with ThreadPool() as pool:
            while True:
                if self.__exit and not self.__clear_queue:
                    self.logger.warning("Forced to exit without clearing the queue.")
                    return
                time.sleep(0.1)
                if not self.__trigger_download(pool) and self.__exit:
                    self.logger.info("Download queue empty. Exiting.")
                    return

    def stop(self, clear_queue: bool = True) -> None:
        """Stop the downloader thread.

        If clear_queue is set to False, then the log downloader will exit ASAP,
        but will wait for ongoing downloads to finish.
        """
        self.__exit = True
        self.__clear_queue = clear_queue

    def __queue_download(self, item: Downloadable) -> None:
        """Queue a downloadable for download."""
        if item.uri not in self.__queued:
            self.__download_queue.put_nowait(item)
            self.__queued.append(item.uri)

    def download_artifacts(self, artifacts: list[Artifact]) -> None:
        """Download artifacts to an artifact path."""
        for artifact in artifacts:
            for _file in artifact.files:
                filepath = self.__artifact_dir.joinpath(f"{artifact.suite_name}").relative_to(
                    Path.cwd()
                )
                filepath.mkdir(exist_ok=True)
                filepath = filepath.joinpath(_file)
                self.__queue_download(
                    Downloadable(uri=f"{artifact.location}/{_file}", name=filepath)
                )

    def download_logs(self, test_suites: Union[list[TestSuite], list[SubSuite]]) -> None:
        """Download logs from test suites to report path."""
        for test_suite in test_suites:
            if not test_suite.finished:
                return
            data = test_suite.finished.get("data", {})
            logs = data.get("testSuitePersistentLogs", [])
            for log in logs:
                filepath = self.__report_dir.joinpath(log["name"]).relative_to(Path.cwd())
                self.__queue_download(Downloadable(uri=log["uri"], name=filepath))
            if isinstance(test_suite, TestSuite):
                self.download_logs(test_suite.sub_suites)
