.. _etos-test-runner-containers:

===========================
ETOS Test Runner Containers
===========================

Github: `ETOS Test Runner Containers <https://github.com/eiffel-community/etos-test-runner-containers>`_

This page describes the available test runner containers.

A test runner container is a docker image with the required tools and commands installed for executing your particular test recipe.

A typical test container is: "etos_python_test_runner" which comes with python pre-installed.


Available test runner containers
================================

Base Test Runner
----------------
The base test runner maintained by the ETOS maintainers. This will include a system and python version that works with the :ref:`etos-test-runner` and will not take into account any other test runner container other than making sure that they work with :ref:`etos-test-runner`.

This means that whoever is responsible for a test runner container makes sure that all dependencies that are required exists within that test runner.

Versioning based on Debian distribution name.

- Latest Debian
- Latest python
- Pyenv for installation and selection of python versions
- :ref:`etos-test-runner`

- eiffel-community/etos-base-test-runner:buster
- eiffel-community/etos-base-test-runner:stretch

Python Test Runner
------------------

Maintainer: :ref:`authors`

Latest Debian and installs python with pyenv

Versioning based on python (examples)

- eiffel-community/etos-python-test-runner:2.7.16
- eiffel-community/etos-python-test-runner:3.5.3
- eiffel-community/etos-python-test-runner:3.7.3
- eiffel-community/etos-python-test-runner:3.8.3

Go Test Runner
--------------

Maintainer: :ref:`authors`

Latest Debian and installs Go.

Versioning based on Go (examples)

- eiffel-community/etos-go-test-runner:1.12.6

Rust Test Runner
----------------

Maintainer: :ref:`authors`

Latest Debian and installs Rust.

Versioning based on Rust (examples)

- eiffel-community/etos-rust-test-runner:1.44.0
