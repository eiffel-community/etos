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
import hashlib
from pathlib import Path
from threading import Thread, Lock
from multiprocessing.pool import ThreadPool
from queue import Queue, Empty
from json import JSONDecodeError

from pydantic import BaseModel
from urllib3.exceptions import MaxRetryError, NewConnectionError
from urllib3.util import Retry
import requests
from requests.exceptions import HTTPError

from etos_lib.lib.http import Http


max_retries = 10  # With 1 as backoff_factor, the total wait time between retries will be 1023 seconds
HTTP_RETRY_PARAMETERS = Retry(
    total=None,
    read=max_retries,
    connect=max_retries,
    status=max_retries,
    backoff_factor=1,
    other=0,
    # 413, 429, 503 + 404 (for cases when the file is not uploaded immediately)
    status_forcelist=list(Retry.RETRY_AFTER_STATUS_CODES) + [404],
)


class Downloadable(BaseModel):
    """Represent a downloadable file."""

    url: str
    name: str
    checksums: dict[str, str] = {}
    path: Path = Path.cwd()


class IntegrityError(Exception):
    """Integrity verification error."""

    def __init__(self, url: str, hash_type: str, downloaded_hash: str, expected_hash: str):
        """Initialize."""
        self.url = url
        self.hash_type = hash_type
        self.downloaded_hash = downloaded_hash
        self.expected_hash = expected_hash

    def __str__(self):
        """Exception as string."""
        return (
            f"Failed integrity protection on {self.url!r}, using hash {self.hash_type!r}. "
            f"Expected: {self.expected_hash!r} but it was {self.downloaded_hash!r}"
        )

    def __repr__(self):
        """Exception as string."""
        return self.__str__()


class Downloader(Thread):  # pylint:disable=too-many-instance-attributes
    """Log downloader for ETOS client."""

    logger = logging.getLogger(__name__)
    started = False

    def __init__(self) -> None:
        """Init."""
        super().__init__()
        self.__download_queue = Queue()
        self.__queued = []
        self.__exit = False
        self.__clear_queue = True
        self.__lock = Lock()
        self.__http = Http(retry=HTTP_RETRY_PARAMETERS)
        self.failed: bool = False
        self.downloads: set[str] = set()

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

    def __save_file(self, item: Downloadable, response: requests.Response) -> None:
        """Save downloaded file data to disk."""
        download_path = item.path.joinpath(item.name)
        if not download_path.parent.exists():
            with self.__lock:
                download_path.parent.mkdir(exist_ok=True, parents=True)
        index = 0
        original_name = download_path.name
        while download_path.exists():
            index += 1
            download_path = download_path.with_name(f"{index}_{original_name}")
        self.logger.debug("Saving file %s", download_path)
        with self.__lock:
            self.downloads.add(str(download_path))
        with open(download_path, "wb+") as report:
            for chunk in response:
                report.write(chunk)
        try:
            self.__verify(item, download_path)
        except IntegrityError:
            download_path.unlink()
            raise

    def __verify(self, item: Downloadable, path: Path):
        """Verify the checksum of the downloaded item.

        Path is the real path to the saved file.
        """
        translation_table = {
            "SHA-224": "sha224",
            "SHA-256": "sha256",
            "SHA-384": "sha384",
            "SHA-512": "sha512",
            "SHA-512/224": "sha512_224",
            "SHA-512/256": "sha512_256",
        }
        hashfunc = None
        expected_digest = None
        for nist_scheme, python_scheme in translation_table.items():
            if item.checksums.get(nist_scheme) is not None:
                hashfunc = hashlib.new(python_scheme)
                expected_digest = item.checksums.get(nist_scheme)
                break

        if hashfunc is None or expected_digest is None:
            self.logger.info("No digest set on file, won't check integrity of %r", item)
            return
        self.logger.debug("Checking integrity of downloaded file using %r", hashfunc.name)

        with path.open("rb") as report:
            hashfunc.update(report.read())
        digest = hashfunc.hexdigest()
        self.logger.debug("Expecting digest %r", expected_digest)
        self.logger.debug("Verify digest %r", digest)

        if digest != expected_digest:
            self.logger.error(
                "%s checksum of file is not as expected. Downloaded: %r , Expected: %r",
                hashfunc.name,
                digest,
                expected_digest,
            )
            raise IntegrityError(item.url, hashfunc.name, digest, expected_digest)
        self.logger.debug("Integrity verification successful")
        return

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
                error_callback=self.__print_traceback,  # type:ignore
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
        # 10 is the default max number of connections in Python requests library
        with ThreadPool(10) as pool:
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

    def queue_download(self, item: Downloadable) -> None:
        """Queue a downloadable for download."""
        if item.url not in self.__queued:
            self.__download_queue.put_nowait(item)
            self.__queued.append(item.url)
