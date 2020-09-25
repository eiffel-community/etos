.. _etos-client:

===========
ETOS Client
===========

Main client for ETOS.

Github: `ETOS Client <https://github.com/eiffel-community/etos-client>`_

This page describes the ETOS client and its options as well as some examples of how it can be used. ETOS Client is a command-line tool that can be used in Continuous Integration tools such as jenkins as well as locally on workstations.

Why ETOS Client?
================

The ETOS client is a tool built to make it easier to execute ETOS (Eiffel Test Orchestration System). It is used for starting test suites in ETOS, and reporting the outcome.

Eager to get started
====================

A getting started page can be found here: :ref:`gettingstarted`

CLI Overview and Command Reference
==================================

Prerequisites
-------------

- Python 3.6 or higher.
- Git (optional)

Installation
------------

There are two methods for installing the package.

Install using pip (Recommended)
*******************************

Recommended for most users. It installs the latest stable version released.
ETOS client can be found on PyPi. If you have the pip package  manager installed, then the simplest way of installing ETOS Client is using the following command:

.. code-block:: bash

   pip install etos_client

Instructions that virtually take you by the hand and guide you every step of the way is available among our :ref:`step-by-step` articles.

Install using Git
*****************

Recommended for developers who want to install the package along with the full source code. Clone the package repository, and install the package in your site-package directory:

.. code-block:: bash

   git clone "https://github.com/eiffel-community/etos-client.git" client
   cd client
   pip install -e .

General Syntax
--------------

The usage example below describes the interface of etos_client, which can be invoked with different combinations of options. The example uses brackets "[ ]" to describe optional elements. Together, these elements form valid usage patterns, each starting with the program's name etos_client.

Below the usage patterns, there is a table of the command-line options with descriptions. They describe whether an option has short/long forms (-h, --help), whether an option has an argument (--identity=<IDENTITY>), and whether that argument has a default value.

.. code-block:: bash

   etos_client [-h] -i IDENTITY -s TEST_SUITE [--no-tty] [-w WORKSPACE] [-a ARTIFACT_DIR] [-r REPORT_DIR] [-d DOWNLOAD_REPORTS] [--iut-provider IUT_PROVIDER] [--execution-space-provider EXECUTION_SPACE_PROVIDER] [--log-area-provider LOG_AREA_PROVIDER] [--dataset DATASET] [--cluster CLUSTER] [--version] [-v]

Command-line options
********************

.. list-table :: ETOS Client
   :widths: 25 25 25
   :header-rows: 1

   * - Option
     - Argument
     - Meaning
   * - -h, --help
     - n/a
     - Show help message and exit.
   * - -i, --identity
     - IDENTITY
     - Artifact created identity purl to execute test suite on.
   * - -s, --test-suite
     - TEST_SUITE
     - URL to test suite where the test suite collection is stored.
   * - --no-tty
     - n/a
     - Disable features requiring a tty.
   * - -w, --workspace
     - WORKSPACE
     - Which workspace to do all the work in.
   * - -a,--artifact-dir
     - ARTIFACT_DIR
     - Where test artifacts should be stored. Relative to workspace.
   * - -r,--report-dir
     - REPORT_DIR
     - Where test reports should be stored. Relative to workspace.
   * - -d,--download-reports
     - DOWNLOAD_REPORTS
     - Should we download reports. Can be 'yes', 'y', 'no', 'n'.
   * - --iut-provider
     - IUT_PROVIDER
     - Which IUT provider to use. Defaults to 'default'.
   * - --execution-space-provider
     - EXECUTION_SPACE_PROVIDER
     - Which execution space provider to use. Defaults to 'default'.
   * - --log-area-provider
     - LOG_AREA_PROVIDER
     - Which log area provider to use. Defaults to 'default'.
   * - --dataset
     - DATASET
     - Additional dataset information to the environment provider. Check with your provider which information can be supplied.
   * - --cluster
     - CLUSTER
     - Cluster should be in the form of URL to ETOS API.
   * - --version
     - n/a
     - Show program's version number and exit
   * - -v, --verbose
     - n/a
     - Set loglevel to DEBUG.

Environment variables
*********************

It is possible to specify an option by using one of the environment variables described below.

Precedence of options
*********************

If you specify an option by using a parameter on the CLI command line, it overrides any value from the corresponding environment variable.

.. list-table :: Optional Environment Variables
   :widths: 25 25 25
   :header-rows: 1

   * - Name
     - Required
     - Meaning
   * - GRAPHQL_API
     - Yes
     - Specifies URL to Eiffel GraphQL API instance to use.
   * - ETOS_API
     - Yes
     - Specifies the URL to the ETOS API for starting tests.
   * - WORKSPACE
     - no
     - Which workspace to do all the work in.
   * - IDENTITY
     - Environment or required input
     - Artifact created identity purl to execute test suite on.
   * - TEST_SUITE
     - Environment or required input
     - URL to test suite where the test suite collection is stored.

Examples
--------

TBD

Known issues
------------

All issues can be found here: https://github.com/eiffel-community/etos-client/issues

Stuck and in need of help
=========================

There is a mailing list at: etos-maintainers@google.groups.com or just write an Issue.
