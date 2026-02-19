#!/usr/bin/env python3
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
import enum
import sys
import time
from subprocess import PIPE, CalledProcessError, CompletedProcess, run
from typing import Optional

MERGEABLE_STATE = enum.Enum("MERGEABLE_STATE", ["UNKNOWN", "CONFLICTING", "MERGEABLE"])
COMMAND = enum.Enum("COMMAND", ["check", "cleanup"])


class Cli:
    """A wrapper around the GitHub CLI to manage pull requests."""

    def __init__(self, repo: str):
        """Initialize the Cli with the given repository."""
        self.repo = repo

    def wait_for_merge_status(
        self, pull_request: str, timeout: int = 60
    ) -> MERGEABLE_STATE:
        """Wait for the status of the pull request to be updated, and return the status."""
        end = time.time() + timeout
        while time.time() < end:
            status = self.get_mergeable_state(pull_request)
            if status and status != MERGEABLE_STATE.UNKNOWN:
                return status
            print(f"Status of pull request {pull_request} is still unknown, waiting...")
            time.sleep(5)
        raise TimeoutError(
            f"Timed out waiting for status of pull request {pull_request} to be updated"
        )

    def get_mergeable_state(self, pull_request: str) -> Optional[MERGEABLE_STATE]:
        """Get the status of a pull request."""
        try:
            proc = self._view(pull_request, "--json", "mergeable", "--jq", ".mergeable")
            output = proc.stdout.decode().strip()
            return MERGEABLE_STATE[output]  # Validate that the output is a valid status
        except CalledProcessError as e:
            print(f"Error running command: {e}")
            return None

    def has_label(self, pull_request: str, label: str) -> bool:
        """Check if a pull request has a specific label."""
        try:
            proc = self._view(
                pull_request, "--json", "labels", "--jq", ".labels[].name"
            )
            labels = proc.stdout.decode().splitlines()
            return label in labels
        except CalledProcessError as e:
            print(f"Error running command: {e}")
        return False

    def add_label(self, pull_request: str, label: str) -> bool:
        """Add a label to a pull request."""
        try:
            self._edit(pull_request, "--add-label", label)
            return True
        except CalledProcessError as e:
            print(f"Error running command: {e}")
            return False

    def remove_label(self, pull_request: str, label: str) -> bool:
        """Remove a label from a pull request."""
        try:
            self._edit(pull_request, "--remove-label", label)
            return True
        except CalledProcessError as e:
            print(f"Error running command: {e}")
            return False

    def pull_requests(self) -> list[str]:
        """Get a list of pull requests in the repository."""
        try:
            proc = self._list()
            pull_requests = proc.stdout.decode().splitlines()
            return [pr.split("\t")[0] for pr in pull_requests]
        except CalledProcessError as e:
            print(f"Error running command: {e}")
            return []
        except IndexError:
            print("No pull requests found")
            return []

    def _edit(self, pull_request: str, *args) -> CompletedProcess:
        """Edit a pull request with the given arguments."""
        cmd = [
            "gh",
            "pr",
            "edit",
            "--repo",
            self.repo,
            pull_request,
            *args,
        ]
        return run(cmd, stdout=PIPE, check=True)

    def _list(self) -> CompletedProcess:
        """List pull requests in the repository."""
        cmd = [
            "gh",
            "pr",
            "list",
            "--repo",
            self.repo,
        ]
        return run(cmd, stdout=PIPE, check=True)

    def _view(self, pull_request: str, *args) -> CompletedProcess:
        """View a pull request with the given arguments."""
        cmd = [
            "gh",
            "pr",
            "view",
            "--repo",
            self.repo,
            pull_request,
            *args,
        ]
        return run(cmd, stdout=PIPE, check=True)


def add_label_if_necessary(gh: Cli, pull_request: str, label: str) -> bool:
    """Add a label to a pull request if it does not already have it."""
    if not gh.has_label(pull_request, label):
        print(f"Pull request {pull_request} does not have label {label}, adding it...")
        return gh.add_label(pull_request, label)
    print(f"Pull request {pull_request} already has label {label}, no need to add it")
    return True


def remove_label_if_necessary(gh: Cli, pull_request: str, label: str) -> bool:
    """Remove a label from a pull request if it has it."""
    if gh.has_label(pull_request, label):
        print(f"Pull request {pull_request} has label {label}, removing it...")
        return gh.remove_label(pull_request, label)
    print(
        f"Pull request {pull_request} does not have label {label}, no need to remove it"
    )
    return True


def check_pull_request(gh: Cli, pull_request: str, label: str) -> bool:
    """Check the status of a pull request and update the label accordingly."""
    try:
        print(f"Checking pull request {pull_request}")
        status = gh.wait_for_merge_status(pull_request)
        print(f"Pull request {pull_request} is {status.name}")
        if status == MERGEABLE_STATE.CONFLICTING:
            return add_label_if_necessary(gh, pull_request, label)
        elif status == MERGEABLE_STATE.MERGEABLE:
            return remove_label_if_necessary(gh, pull_request, label)
        else:
            print(f"Pull request {pull_request} has unknown mergeable state")
            return False
    except TimeoutError as e:
        print(e)
        return False


def main(command: COMMAND, repository: str, label: str):
    """Main function to check or clean up labels on pull requests."""
    gh = Cli(repository)
    for pull_request in gh.pull_requests():
        if command == COMMAND.check:
            if not check_pull_request(gh, pull_request, label):
                print(f"Failed to check pull request {pull_request}")
                raise SystemExit(1)
        elif command == COMMAND.cleanup:
            if not remove_label_if_necessary(gh, pull_request, label):
                print(f"Failed to clean up labels on pull request {pull_request}")
                raise SystemExit(1)


if __name__ == "__main__":
    CMD = COMMAND[sys.argv[1]]
    REPOSITORY = sys.argv[2]
    LABEL = sys.argv[3]
    main(CMD, REPOSITORY, LABEL)
