.. _release_process:

===============
Release Process
===============

The ETOS release process is as follows.

All ETOS components will have their own release process using continuous integration.
This means that all components will have different releases at all times.

Every other wednesday (uneven weeks) the ETOS project will get a new tag and a new release and all helm charts will be updated with the new component versions.

When there are breaking changes in ETOS upcoming, there will be milestones with deadlines for each stage of a breaking change.
For backwards compatibility we will first deprecate the feature with a feature flag to disable it.
We will create issues for changing the default behavior to disabled two weeks from the deprecation release.
We will also create issues for removing the feature completely two weeks from the disable release.

This means that, at worst, users get a 4 week heads up for breaking changes. And by breaking changes, we mean that a developer may have to change something in their code or :ref:`tercc` before they can upgrade.

Each milestone is considered a new release with a version number (:ref:`either minor or major <versioning>`) 