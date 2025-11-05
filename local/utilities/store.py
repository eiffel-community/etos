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
"""Local store."""
from typing import Any


class Store:
    """Store variables into a dictionary-like object.

    When getting items from this store a `Value` is returned
    instead.
    This Store is used in commands before they are executed in order
    to get runtime information from previous commands.

    For example

        local_store = Store()
        cmds = [
            StoreStdout(Shell(["ls", "-lah"]), key="files", store=local_store)
            Shell(["echo", local_store["files"]])
        ]
        for cmd in cmds:
            cmd.execute()
        # The second command (Shell(["echo"...])) will echo the value stored
        # in local_store["files"] which was saved by StoreStdout after .execute()
    """

    def __init__(self):
        self.__store = {}

    def __setitem__(self, key: Any, value: Any):
        self.__store[key] = value

    def __getitem__(self, key) -> "Value":
        return Value(self.__store, key)

    def __repr__(self) -> str:
        return str(self.__store)


class Value:
    """Value variable that is stored in Store.

    Must call `.get()` or, if it's a string, `str(Value)` would
    also work in most cases.
    """

    def __init__(self, store: dict[Any, Any], key: Any):
        self.__store = store
        self.__key = key

    def get(self) -> Any:
        return self.__store.get(self.__key)

    def __str__(self) -> str:
        return str(self.__store.get(self.__key))

    def __repr__(self) -> str:
        return f"<Value key={self.__key}>"


def get_value(value: Any) -> Any:
    if isinstance(value, Value):
        return value.get()
    return value


def get_values(*values: Any) -> list[Any]:
    return [get_value(value) for value in values]
