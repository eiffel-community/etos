.. _versioning:

################
Versioning rules
################

ETOS uses semantic versioning in the form of "MAJOR.MINOR.PATCH"

- MAJOR: A large change, sometimes breaking changes.
- MINOR: A smaller change, never a breaking change.
- PATCH: A bugfix required in production.

Whenever a new ETOS MAJOR or MINOR deployment is made, we _ALWAYS_ deploy every tool and all versions will be the same in each service (at the time of release).

PATCH updates in productions are allowed to be deployed on individual services but the PATCH number must also be incremented in the 'ETOS' repository. The ETOS repository holds the main helm chart and the main version number.
A PATCH is ALWAYS a small bugfix to a specific component that is REQUIRED for production to work.

Examples

If we release version 1.0.0 of ETOS:

- etos: 1.0.0
- etos-client: 1.0.0
- etos-api: 1.0.0
- etos-suite-starter: 1.0.0
- etos-suite-runner: 1.0.0
- etos-test-runner: 1.0.0
- etos-environment-provider: 1.0.0
- etos-library: 1.0.0
- etos-test-runner-containers: 1.0.0

Then we notice a bug in the "etos-client" which makes it impossible to run ETOS. We fix this bug and increase the PATCH number and release it:

- etos: 1.0.1
- etos-client: 1.0.1
- etos-api: 1.0.0
- etos-suite-starter: 1.0.0
- etos-suite-runner: 1.0.0
- etos-test-runner: 1.0.0
- etos-environment-provider: 1.0.0
- etos-library: 1.0.0
- etos-test-runner-containers: 1.0.0

Now we notice a bug in the "etos-environment-provider". Let's fix it and release.

- etos: 1.0.2
- etos-client: 1.0.1
- etos-api: 1.0.0
- etos-suite-starter: 1.0.0
- etos-suite-runner: 1.0.0
- etos-test-runner: 1.0.0
- etos-environment-provider: 1.0.1
- etos-library: 1.0.0
- etos-test-runner-containers: 1.0.0


Note that the PATCH number of ETOS increases everytime and that we are not increasing the PATCH number of every tool.

Now if we make a MINOR release of ETOS, this will happen with the versions:

- etos: 1.1.0
- etos-client: 1.1.0
- etos-api: 1.1.0
- etos-suite-starter: 1.1.0
- etos-suite-runner: 1.1.0
- etos-test-runner: 1.1.0
- etos-environment-provider: 1.1.0
- etos-library: 1.1.0
- etos-test-runner-containers: 1.1.0

If you want all PATCH releases for the current MINOR release: ">= 1.1.0 < 1.2.0"

If you want all MINOR releases: ">= 1.0.0 < 2.0.0"

We do not recommend you running latest at all times, since we can break backwards compatability between MAJOR releases.
Breaking changes WILL BE announced ahead of time.
