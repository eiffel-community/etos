.. _versioning:

################
Versioning rules
################

ETOS uses semantic versioning in the form of "MAJOR.MINOR.PATCH"

- MAJOR: A large change, sometimes breaking changes.
- MINOR: A smaller change, never a breaking change.
- PATCH: A bugfix required in production.

Each ETOS component have their own versioning and will be selected for an ETOS release every other wednesday.

The ETOS helm charts might get a PATCH update outside of the regular release schedule if there are breaking changes that need to be patched in production.

If you want all PATCH releases for the current MINOR release: ">= 1.1.0 < 1.2.0"

If you want all MINOR releases: ">= 1.0.0 < 2.0.0"

We do not recommend you running latest at all times, since we can break backwards compatability between MAJOR releases.
Breaking changes WILL BE announced ahead of time.
