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
from typing import Union
from threading import Thread, Lock
from multiprocessing.pool import ThreadPool
from queue import Queue, Empty
from json import JSONDecodeError

from pydantic import BaseModel
from urllib3.exceptions import MaxRetryError, NewConnectionError
from urllib3.util import Retry
import requests
from requests.exceptions import HTTPError
import jmespath

from etos_lib.lib.http import Http


HTTP_RETRY_PARAMETERS = Retry(
    total=None,
    read=0,
    connect=10,  # With 1 as backoff_factor, will retry for 1023s
    status=10,  # With 1 as backoff_factor, will retry for 1023s
    backoff_factor=1,
    other=0,
    # 413, 429, 503 + 404 (for cases when the file is not uploaded immediately)
    status_forcelist=list(Retry.RETRY_AFTER_STATUS_CODES) + [404],
)


class Filter(BaseModel):
    """Filter for logs and artifacts."""

    source: str
    jmespath: str


class Downloadable(BaseModel):
    """Represent a downloadable file."""

    url: str
    name: list[Filter]
    path: Path = Path.cwd()


class Directory(BaseModel):
    """Represent a directory of files."""

    name: str
    logs: list[Downloadable]
    artifacts: list[Downloadable]


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
        self.__http = Http(retry=HTTP_RETRY_PARAMETERS)
        self.failed: bool = False
        self.downloads: {str} = set()

        self.__report_dir = report_dir
        self.__artifact_dir = artifact_dir

    def __download(self, item: Downloadable) -> None:
        """Download files."""
        self.logger.debug("Downloading %r", item)
        # retry rules are set in the Http client
        response = self.__http.get(item.url, stream=True)
        if self.__download_ok(response):
            self.__save_file(item, response)
            self.logger.debug("Item downloaded %r", item)
            return
        with self.__lock:
            self.failed = True
        self.logger.critical("Failed to download %r", item)
        return

    def __apply_filter(self, item: Downloadable, response: requests.Response) -> str:
        """Apply name filter to downloaded item."""
        name = []
        try:
            response_json = (
                response.json()
                if response.headers.get("content-type") == "application/json"
                else None
            )
        except JSONDecodeError:
            # If a file ends in .json it will be interpreted as json, but it might not be valid.
            response_json = None
        sources = {"response": response_json, "headers": response.headers}

        for name_filter in item.name:
            source = sources.get(name_filter.source)
            if source is None:
                raise ValueError(f"Filter source {name_filter.source} not found.")
            name.append(jmespath.search(name_filter.jmespath, source))
        return "/".join(name)

    def __save_file(self, item: Downloadable, response: requests.Response) -> None:
        """Save downloaded file data to disk."""
        download_name = self.__apply_filter(item, response)
        download_path = item.path.joinpath(download_name)
        if not download_path.parent.exists():
            with self.__lock:
                download_path.parent.mkdir(exist_ok=True, parents=True)
        index = 0
        while download_path.exists():
            index += 1
            download_path = download_path.with_name(f"{index}_{download_path.name}")
        self.logger.debug("Saving file %s", download_path)
        with self.__lock:
            self.downloads.add(str(download_path))
        with open(download_path, "wb+") as report:
            for chunk in response:
                report.write(chunk)

    def __download_ok(self, response: requests.Response) -> bool:
        """Check download response and log response details."""
        try:
            response.raise_for_status()
            return True
        except HTTPError as http_error:
            if http_error.response.status_code == 404:
                self.logger.critical("File not found")
                return False
            try:
                response_json = response.json()
            except JSONDecodeError:
                self.logger.debug("Raw HTTP response from download: %r", response.text)
                response_json = {
                    "detail": "Unknown HTTP client error when downloading files from log area"
                }
            self.logger.critical("Download response: %r", response_json)
            return False
        except (
            ConnectionError,
            NewConnectionError,
            MaxRetryError,
            TimeoutError,
        ):
            self.logger.exception("Network connectivity error when downloading logs and artifacts.")
            return False

    def __trigger_download(self, pool: ThreadPool) -> bool:
        """Get downloadable from queue and add it to threadpool."""
        try:
            item = self.__download_queue.get_nowait()
            pool.apply_async(
                self.__download,
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
        if item.url not in self.__queued:
            self.__download_queue.put_nowait(item)
            self.__queued.append(item.url)

    def __download_artifacts(self, artifacts: list[Downloadable], path: Path) -> None:
        """Download artifacts to an artifact path."""
        for artifact in artifacts:
            artifact.path = path
            self.__queue_download(artifact)

    def __download_logs(self, logs: list[Downloadable], path: Path) -> None:
        """Download logs from test suites to report path."""
        for log in logs:
            log.path = path
            self.__queue_download(log)

    def download_files(self, directory: Directory):
        """Download logs and artifacts from a directory."""
        reports = self.__report_dir.relative_to(Path.cwd())
        artifacts = self.__artifact_dir.relative_to(Path.cwd()).joinpath(directory.name)
        self.__download_logs(directory.logs, reports)
        self.__download_artifacts(directory.artifacts, artifacts)

    def download_directories(self, directories: dict):
        """Download logs and artifacts from directories."""
        for name, directory in directories.items():
            self.download_files(Directory(name=name, **directory))
