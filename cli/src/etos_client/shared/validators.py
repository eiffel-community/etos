# -*- coding: utf-8 -*-
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
"""Validators for CLI arguments of the start subcommand."""

import uuid
from typing import Any, Callable, Dict


class CLIArgsValidationError(Exception):
    """Exception raised when start subcommand argument validation fails."""
    pass


class IdentityValidator:
    """Validator specifically for identity arguments (PURL or UUID)."""
    
    @staticmethod
    def validate(value: str) -> None:
        """Validate that a value is either a valid PURL or UUID."""
        error_message = "Invalid identity: '{value}'. Expected: either a valid UUID or PURL (pkg:...)"

        if not value or not isinstance(value, str):
            raise CLIArgsValidationError(error_message.format(value=value))
        
        # Try to validate as UUID first
        if IdentityValidator._is_valid_uuid(value):
            return
        
        # Try to validate as PURL
        if IdentityValidator._is_valid_purl(value):
            return
        
        raise CLIArgsValidationError(error_message.format(value=value))
    
    @staticmethod
    def _is_valid_uuid(value: str) -> bool:
        """Check if a string is a valid UUID."""
        try:
            uuid.UUID(value)
            return True
        except (ValueError, AttributeError):
            return False
    
    @staticmethod
    def _is_valid_purl(value: str) -> bool:
        """Check if a string is a valid Package URL (PURL)."""
        # For now, just validate that it starts with "pkg:"
        # This can be improved later with more comprehensive validation
        return value.startswith("pkg:")


class StartSubcommandArgumentValidator:
    """Extensible argument validator for the start subcommand CLI arguments."""
    
    def __init__(self):
        """Initialize the validator with built-in validation rules for start subcommand arguments."""
        self._validators: Dict[str, Callable[[Any], None]] = {
            'identity': IdentityValidator.validate,
        }
    
    def add_validator(self, argument_name: str, validator_func: Callable[[Any], None]) -> None:
        """Add a custom validator for a start subcommand argument."""
        self._validators[argument_name] = validator_func
    
    def validate(self, argument_name: str, value: Any) -> None:
        """Validate an argument value."""
        if argument_name in self._validators:
            self._validators[argument_name](value)
    
    def validate_args(self, args: Dict[str, Any], argument_mappings: Dict[str, str] = None) -> None:
        """Validate multiple start subcommand arguments from a parsed arguments dictionary."""
        if argument_mappings is None:
            argument_mappings = {
                '--identity': 'identity',
                '-i': 'identity',
            }
        
        for arg_key, validator_name in argument_mappings.items():
            if arg_key in args and args[arg_key] is not None:
                self.validate(validator_name, args[arg_key])


start_args_validator = StartSubcommandArgumentValidator()