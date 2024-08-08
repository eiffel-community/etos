ETOS Controller
===============

Cluster
-------

A resource for deploying a new ETOS cluster. Can be configured to deploy a full environment that is usable out of the box.

TestRun
-------

A resource for describing an ETOS testrun. This is the test suite part of ETOS and will trigger an execution of the test suite.
The TestRun resource is similar, but not identical, to the test suite argument to ETOS API. The TestRun specification is more ETOS
focused while the test suite argument to the ETOS API is more focused on Eiffel (specifically the TERCC).
The goal is for ETOS to use the TestRun specification instead of the Eiffel-based one in the future, for now we convert between them.

Environment
-----------

Describes the environment where a specific TestRun is executed. Will have information about IUTs, execution spaces and log areas as
well as configurations for them. There's also status fields that can be used to view the current status of the environment.
An Environment is always connected to a TestRun.

Provider
--------

A provider describes a particular provider, be it IUT, execution space or log area. The specification of the provider contains
instructions on how to use the provider and it also has a status field where you can view the status of the provider.
A provider is not directly connected to a TestRun or Environment, but is referenced by the two.
