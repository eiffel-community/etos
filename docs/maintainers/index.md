<!---
   Copyright Axis Communications AB
   For a full list of individual contributors, please see the commit history.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
--->

The following document is aimed at maintainers and contributors to the ETOS project. If you are a user of ETOS, please refer to the [Documentation](../index.md) for more information on how to use ETOS.

# Release process

There are two types of "tracks" for ETOS releases: stable and development.
The stable track is for releases that are considered stable and ready for production use, while the development track is for releases that are still in development and may contain breaking changes.

When an ETOS service has any change merged a bot will automatically create a pull request to update the default version of that service in the [ETOS](https://github.com/eiffel-community/etos) repository. This pull request can then be merged by a maintainer at their leisure.
If you have contributed by writing code for an ETOS service, you can track this pull request to see when your contribution will be included.

The [ETOS test runner](https://github.com/eiffel-community/etos-test-runner) is a special case since it is built as a python package and uploaded only when a new release is created in the ETOS test runner repository. This means that in order for changes from the test runner to become available a new release must be created.
After the release has been created a pull request is still automatically created to update the default version of the test runner in the ETOS repository.

As soon as the pull request is merged, the new version of the ETOS controller is built and is made available in the development track by applying the [install.yaml](https://raw.githubusercontent.com/eiffel-community/etos/refs/heads/main/manifests/controller/install.yaml) file from manifest folder.
This means that the new version of the ETOS controller is available for testing and can be used to run testruns with the new features or bug fixes.

Once the new version of ETOS has been tested and is considered stable, a maintainer can create a new release on github, setting a new git tag, and a stable version is made available from the release ([example](https://github.com/eiffel-community/etos/releases/latest/download/install.yaml)).

## Development steps

### Development release steps
1. Create a pull request with your changes to the ETOS service (for example the [ETOS API](https://github.com/eiffel-community/etos-api)).
2. Wait for the pull request to be merged.
    1. If you have contributed to the ETOS test runner, create a new release in the [ETOS test runner repository](https://github.com/eiffel-community/etos-test-runner) to upload the new version of the test runner to PyPI.
3. Wait for the bot to create a pull request to update the default version of the service in the ETOS repository.
4. Wait for the pull request to be merged.
5. The new version of the ETOS controller is now available in the development track and can be used for testing.

### Stable release steps
1. Once the new version is considered stable, create a new release on github, setting a new git tag.
2. The new stable version of ETOS is now available and can be used for production use.


# Versioning

ETOS follows semantic versioning, which means that the version number is in the format of `MAJOR.MINOR.PATCH`.

- MAJOR version is incremented when there are breaking changes.
- MINOR version is incremented when there are new features that are backwards compatible.
- PATCH version is incremented when there are bug fixes that are backwards compatible.

There is no versioning on the individual services (except for the ETOS test runner), but the version of the ETOS controller is updated to reflect the changes in the services. For example, if there is a new feature added to the ETOS API, the version of the ETOS controller will be updated to reflect that change.
