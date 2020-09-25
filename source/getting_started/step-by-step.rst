.. _step-by-step:

============
Step by step
============


Installing ETOS Client on a workstation
=======================================

This section will guide you through the process of setting up :ref:`etos-client`. 

:ref:`etos-client` is the default tool for executing the test suites with. We always recommend using the client.

Requirements
------------

- `Python <https://www.python.org>`_ 3.6 (or higher)

Installation
------------

:ref:`etos-client` can be found on PyPi and is installable with pip.

.. code-block:: bash

   pip install etos_client


CLI Usage
---------

.. code-block:: bash

   etos_client --help

More on usage can be found :ref:`here<services/etos_client:General Syntax>`


Setting up a Jenkins delegation job
===================================

This page describes how to set up delegation jobs for ETOS.
A delegation job's function is described :ref:`here <services/etos_environment_provider:Execution Space Provider>`

Note that a delegation job can be created just the way you want to (as long as it follows the instructions from the execution space), this is just a sample of how you could implement it.

Prerequisites
-------------

- `Jenkins <https://www.jenkins.io>`_
- `Jenkins Pipelines <https://www.jenkins.io/doc/book/pipeline/>`_

Example setup
-------------

#. Create a pipeline job.
#. Recommended to set cleanup policy for the job.
#. Add multi-line string parameter named 'docker'.
#. Configure :ref:`Execution Space <services/etos_environment_provider:Execution Space Provider>` to send the 'docker' parameter to Jenkins.
#. Add script to delegation

.. code-block:: groovy

   node() {
       stage('ETOS') {
           def jsonslurper = new groovy.json.JsonSlurper()
           def json = params.docker
           def dockerJSON = jsonslurper.parseText(json)
           
           def environmentJSON = dockerJSON["environment"]
           def parametersJSON = dockerJSON["parameters"]
           def dockerName
           if (parametersJSON.containsKey("--name")) {
               dockerName = parametersJSON["--name"]
           } else {
               dockerName = UUID.randomUUID().toString()
               parametersJSON["--name"] = dockerName
           }
           env.DOCKERNAME = dockerName
           def environment = ""
           def parameters = ""
           environmentJSON.each{entry -> environment += "-e $entry.key=$entry.value "}
           parametersJSON.each{entry -> parameters += "$entry.key $entry.value "}
           def image = dockerJSON["image"]
           def command = "docker run --rm " + environment + parameters + image + " &"
           /*
             Write a bash file which will trap interrupts so that the docker container
             is properly removed when canceling a build.
           */
           writeFile file: 'run.sh', text: (
               '_terminate() {\n'
               + '    echo "Stopping container"\n'
               + "    docker stop $dockerName\n"
               + '}\n'
               + 'trap _terminate SIGTERM\n'
               + "$command \n"
               + 'child=$!\n'
               + 'wait "$child"\n'
           )
           sh "docker pull $image || true"
           sh """
           bash run.sh
           docker rm $dockerName || true
           """
           sh "rm run.sh"
       }
   }


Example execution space provider
--------------------------------

Checkout any number of static execution spaces.
More information about execution space providers :ref:`here <services/etos_environment_provider:Execution Space Provider>`

.. code-block:: json

   {
     "execution_space": {
           "id": "jenkins",
           "list": {
               "possible": {
                   "$expand": {
                       "value": {
                           "request": {
                               "url": "https://jenkins/job/DELEGATION/build",
                               "method": "POST",
                               "headers": {
                                   "Accept": "application/json"
                               },
                               "data": {
                                   "json": {
                                       "$json_dumps": {
                                           "parameter": [
                                               { "name": "docker", "value": {
                                                   "$json_dumps": "$execution_space_instructions"
                                                 }
                                               }
                                           ]
                                       }
                                   }
                               }
                           }
                       },
                       "to": "$amount"
                   }
               },
               "available": "$this.possible"
           }
       }
   }
