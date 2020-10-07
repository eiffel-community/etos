.. _execution_space:


===============
Execution Space
===============


An execution space is a system where docker containers can run.

In the ETOS world the execution space is responsible for receiving a run order where it will start a docker container using the instructions from the :ref:`etos-environment-provider`.

This execution space will then execute the docker container until completion and then shut down.

An example of an execution space is `Jenkins <https://www.jenkins.io>`_
