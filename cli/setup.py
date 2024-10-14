# -*- coding: utf-8 -*-
"""Setup file for ETOS Client."""
from setuptools import setup
from setuptools_scm.version import get_local_dirty_tag


def version_scheme(version) -> str:
    """Get version component for the current commit.

    Used by setuptools_scm.
    """
    if version.tag and version.distance == 0:
        # If the current commit has a tag, use the tag as version, regardless of branch.
        # Note: Github CI creates releases from detached HEAD, not from a particular branch.
        return f"{version.tag}"
    elif version.branch == "main" and version.tag and version.distance > 0:
        # For untagged commits on the release branch always add a distance like ".post3"
        return f"{version.tag}.post{version.distance}"
    else:
        # For non-release branches, mark the version as dev and distance:
        return f"{version.tag}.dev{version.distance}"


def local_scheme(version) -> str:
    """Get local version component for the current Git commit.

    Used by setuptools_scm.
    """
    # If current version is dirty, always add dirty suffix, regardless of branch.
    dirty_tag = get_local_dirty_tag(version) if version.dirty else ""
    if dirty_tag:
        return f"{dirty_tag}.{version.node}"

    if version.distance == 0:
        # If the current commit has a tag, do not add a local component, regardless of branch.
        return ""
    # For all other cases, always add the git reference (like "g7839952")
    return f"+{version.node}"


if __name__ == "__main__":
    setup(
        use_scm_version={
            "root": "..",
            "relative_to": __file__,
            "local_scheme": local_scheme,
            "version_scheme": version_scheme,
        }
    )
