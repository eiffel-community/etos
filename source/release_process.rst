.. _release_process:

===============
Release Process
===============

The ETOS release process is as follows.

ETOS will do a full release every other wednesday (uneven weeks).
This release will be done at any time that day and it will include updating all docker containers, updating the helm chart, creating a tag and release on every repository on github.

The ETOS workflow is to work with milestones.

Each milestone is considered a new release with a version number (:ref:`either minor or major <versioning>`) and the issues that are solved in milestones will be added to that release.
(Note that we might hold back issues for a release if they are not deemed stable enough, but we will communicate this by removing the issue from the milestone).

These miletones are updated and created every friday and there will always be at least two future milestones in the works so that users have a heads-up what will be coming in the future.

If backwards compatibility is broken then the feature will be deprecated, the new one activated and an issue for removing it will be added to a major milestone in the future so that users know ahead of time whene a breaking change is coming.

This means that, at worst, users get a 4 week heads up for breaking changes. And by breaking changes, we mean that a developer may have to change something in their code or :ref:`tercc` before they can upgrade.


In other words:

* New minor or major release on wednesday, uneven weeks.
* Milestones communicate new releases ahead of time with issues.
* Impact of issues decide whether milestone is major or minor.
* Milestones are created and updated every friday.
* There will always be at least two milestones active for two and four weeks in the future, respectively.
