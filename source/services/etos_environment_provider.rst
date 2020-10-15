.. _etos-environment-provider:

=========================
ETOS Environment Provider
=========================

Github: `ETOS Environment Provider <https://github.com/eiffel-community/etos-environment-provider>`_

The ETOS environment provider is used to fetch an environment in which to execute test automation on. An environment is the combination of :ref:`IUTs <services/etos_environment_provider:IUT Provider>`, :ref:`Log areas <services/etos_environment_provider:Log Area Provider>` and :ref:`Execution spaces <services/etos_environment_provider:Execution Space Provider>`.

Each provider is in the form of :ref:`services/etos_environment_provider:JSONTas` and must be registered with a name in the environment provider before starting any tests.
The ETOS environment provider can have multiple providers registered for each role and a provider can be chosen as an input parameter to :ref:`ETOS Client <services/etos_client:Command-line options>`.

It is recommended to add one provider of each type with the name 'default' in order to make it easier for the user (as they don't have to supply a name to :ref:`etos-client`.


JSONTas
=======

The ETOS environment provider uses `JSONTas <https://jsontas.readthedocs.io>`_ in order to structure the providers. Static JSON is still supported (JSONTas just won't do anything).

A JSONTas structure can be quite complex but is useful for generating dynamic JSON structures based on several factors. More examples on JSONTas can be found in their `examples <https://jsontas.readthedocs.io/en/latest/examples.html>`_

There are a few built-in datastructures in the environment provider as well in order to make life easier.

- json_dumps: Dump an entire segment to string.
- uuid_generate: Generate a UUID4 string
- join: Join a list of strings together.

For the :ref:`Execution Space Provider <services/etos_environment_provider:Execution Space Provider>` there is another data structure.

- execution_space_instructions: Used to override the instructions or add more instructions for the :ref:`execution_space`.


General Structure
=================

The general structure of a provider is comrised of at least 3 different main parts.

- List
- Checkout
- Checkin

List
----

List is where we list the possible and available items.
Possible is the number of possible items. If it's 0 then the environment provider will exit with
"Could not checkout".

If possible>0 but available is 0, that means there are items possible to checkout but they are not available yet and the environment provider will wait until they are (with a maximum limit).

Note that the listing can be a request to a management system or just a static list of items, but both the 'available' and 'possible' keys must be set.

Checkout
--------

An optional parameter in the provider structure. It's a command which will 'checkout' the item from a management system, if there is one available.

The checkout can also return more values to add to the resulting JSON.

Checkin
-------

An optional parameter in the provider structure. It's a command which will 'check in' the item to a management system, making the item available for others.


IUT Provider
============

An IUT provider returns a list of JSON data describing an :ref:`iut` (Item Under Test).

The IUT provider follows the :ref:`general structure <services/etos_environment_provider:General Structure>` of 'list', 'checkout' and 'checkin' but also adds another part which is the 'prepare' part.

Prepare
-------

The prepare part of the IUT Provider is defined with stages and steps. A stage is 'where shall this preparation run' and the step is 'what should we run to prepare the IUT'.

There is currently only a single 'stage' and that stage is 'environment_provider' which is run just after the 'checkout' step in the provider.

Each step is a key, value pair where the key is the name of the step and the value is a :ref:`services/etos_environment_provider:JSONTas` structure.

A sample preparation step which will execute three steps. One where the return value is a dictionary, one where the return value is a part of the previous step and the third requests a webpage.

Note that this example does not do anything with the IUT. It is virtually impossible for us to describe the steps required for your technology domain as it all depends on how your systems are set up.
This preparation step can request APIs that you've set up internally for various scenarios.

.. code-block:: json

   {
      "prepare": {
         "stages": {
            "environment_provider": {
               "steps": {
                  "step1": {
                     "something": "text",
                     "another": "text2"
                  },
                  "step2": {
                     "previous_something": "$steps.step1.something"
                  },
                  "step3": {
                     "$request": {
                        "url": "https://jsonplaceholder.typicode.com/users/1",
                        "method": "GET"
                     }
                  }
               }
            }
         }
      }
   }


Example
-------

A single static :ref:`iut`

.. code-block:: json

   {
       "iut": {
            "id": "default",
            "list": {
                 "possible": [
                     {
                          "type": "$identity.type",
                          "namespace": "$identity.namespace",
                          "name": "$identity.name",
                          "version": "$identity.version",
                          "qualifiers": "$identity.qualifiers",
                          "subpath": "$identity.subpath"
                     }
                  ],
                 "available": "$this.possible"
            }
       }
   }

Using a management system

.. code-block:: json

   {
       "iut": {
           "id": "mymanagementsystem",
           "checkout": {
               "$condition": {
                   "then": {
                       "$request": {
                           "url": "http://managementsystem/checkout",
                           "method": "GET",
                           "params": {
                               "mac": "$identity.name"
                           }
                       }
                   },
                   "if": {
                       "key": "$response.status_code",
                       "operator": "$eq",
                       "value": 200
                   },
                   "else": "$response.json.message"
               }
           },
           "checkin": {
               "$operator": {
                   "key": {
                       "$from": {
                           "item": {
                               "$request": {
                                   "params": {
                                       "id": "$iut.id"
                                   },
                                   "url": "http://managemenetsystem/checkin",
                                   "method": "GET"
                               }
                           },
                           "get": "id"
                       }
                   },
                   "operator": "$eq",
                   "value": "$iut.id"
               }
           },
           "list": {
               "possible": {
                   "$request": {
                       "url": "http://managementsystem/list",
                       "method": "GET",
                       "params": {
                           "name": "$identity.name"
                       }
                   }
               },
               "available": {
                   "$filter": {
                       "items": "$this.possible",
                       "filters": [
                           {
                               "key": "checked_out",
                               "operator": "$eq",
                               "value": "false"
                           }
                       ]
                   }
               }
           }
       }
   }

With a preparation step

.. code-block:: json

   {
       "iut": {
            "id": "default",
            "list": {
                 "possible": [
                     {
                          "type": "$identity.type",
                          "namespace": "$identity.namespace",
                          "name": "$identity.name",
                          "version": "$identity.version",
                          "qualifiers": "$identity.qualifiers",
                          "subpath": "$identity.subpath"
                     }
                  ],
                 "available": "$this.possible"
            },
            "prepare": {
               "stages": {
                  "environment_provider": {
                     "steps": {
                        "step1": {
                           "something": "text",
                           "another": "text2"
                        },
                        "step2": {
                           "previous_something": "$steps.step1.something"
                        },
                        "step3": {
                           "$request": {
                              "url": "https://jsonplaceholder.typicode.com/users/1",
                              "method": "GET"
                           }
                        }
                     }
                  }
               }
            }
       }
   }


Log Area Provider
=================

A log area provider makes sure that the ETOS system knows where and how to store logs and test artifacts during and after execution.

A :ref:`log area <log_area>` has several parts that must exist within the resulting :ref:`log area <log_area>` definition (after listing and checking out)

- livelogs (required): A path to where live logs from the system can be viewed. Used in the test suite events.
- upload (required): How to upload logs to the :ref:`log area <log_area>` system. Follows the same syntax as JSONTas `requests <https://jsontas.readthedocs.io/en/latest/api/jsontas.data_structures.html#jsontas.data_structures.request.Request>`_ (without the '$' signs)
- logs (optional): Extra formatting on logs.
   - prepend: an extra value to prepend to log files.
   - join_character: With which character to join the prepended data. Default: '_'


Example using JFrog Artifactory.
Checkout any number of artifactory instances, storing logs in a folder based on the Eiffel context.
Also prepend IP Address if the :ref:`iut` has an 'ip_address' property.

.. code-block:: json

   {
       "log": {
           "id": "artifactory",
           "list": {
               "possible": {
                   "$expand": {
                       "value": {
                           "livelogs": {
                                   "$join": {
                                           "strings": [
                                                   "https://artifactory/logs/",
                                                   "$context"
                                           ]
                                   }
                           },
                           "upload": {
                                   "url": {
                                           "$join": {
                                                   "strings": [
                                                           "https://artifactory/logs/",
                                                           "$context",
                                                           "/{folder}/{name}"
                                                   ]
                                           }
                                   },
                                   "method": "PUT",
                                   "auth": {
                                           "username": "user",
                                           "password": "password",
                                           "type": "basic"
                                   }
                           },
                           "logs": {
                                   "$condition": {
                                           "if": {
                                                   "key": "$iut.ip_address",
                                                   "operator": "$regex",
                                                   "value": "^(?:[0-9]{1,3}\\.){3}[0-9]{1,3}$"
                                           },
                                           "then": {
                                                   "prepend": "$iut.ip_address"
                                           },
                                           "else": {}
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


Execution Space Provider
========================

An execution space provider makes sure that the ETOS system knows where it can start the :ref:`etos-test-runner`.
The :ref:`execution space <execution_space>` must have one required key, which is the 'request' key. This key is the description of how the :ref:`etos-suite-runner` can start the :ref:`etos-test-runner` instance.

There is also a field called 'execution_space_instructions' that is dynamically created every time and can be overriden if more information needs to be added. These instructions are the instructions for how to execute the :ref:`etos-test-runner` docker container.


Example of a Jenkins execution space provider

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
                                               {"name": "docker", "value": {
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


Overriding the execution space instructions (note that the '$json_dumps' value has changed).

.. code-block:: json

   {
     "execution_space": {
           "id": "jenkins",
           "list": {
               "possible": {
                   "$expand": {
                       "value": {
                           "instructions": {
                              "$execution_space_instructions": {
                                 "environment": {
                                    "MYENV": "environment variable"
                                 },
                                 "parameters": {
                                    "--privileged": ""
                                 }
                              }
                           },
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
                                               {"name": "docker", "value": {
                                                   "$json_dumps": "$expand_value.instructions"
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


The default instructions are as follows (all can be overriden):

.. code-block:: python

   instructions = {
       "image": self.dataset.get("test_runner"),
       "environment": {
           "RABBITMQ_HOST": rabbitmq.get("host"),
           "RABBITMQ_USERNAME": rabbitmq.get("username"),
           "RABBITMQ_PASSWORD": rabbitmq.get("password"),
           "RABBITMQ_EXCHANGE": rabbitmq.get("exchange"),
           "RABBITMQ_PORT": rabbitmq.get("port"),
           "RABBITMQ_VHOST": rabbitmq.get("vhost"),
           "SOURCE_HOST": self.etos.config.get("source").get("host"),
           "ETOS_GRAPHQL_SERVER": self.etos.debug.graphql_server,
           "ETOS_API": self.etos.debug.etos_api,
           "ETOS_ENVIRONMENT_PROVIDER": self.etos.debug.environment_provider,
       },
       "parameters": {}
   }
   instructions["identifier"] = str(uuid4())
   instructions["environment"]["SUB_SUITE_URL"] = "{}/sub_suite?id={}".format(
      instructions["environment"]["ETOS_ENVIRONMENT_PROVIDER"],
      instructions["identifier"],
   )


This is a great place to get value from the optional :ref:`dataset <services/etos_environment_provider:Dataset>` that can be passed to :ref:`etos-client`.
The dataset is always added as a dataset to JSONTas and any value can be referenced with the '$' notation in the JSONTas provider files.

Note that the dataset can be added to any part of the JSON files. See :ref:`here <services/etos_environment_provider:Dataset>` for more examples

.. code-block:: json

   {
      "instructions": {
         "$execution_space_instructions": {
            "environment": {
               "MYENV": "$my_dataset_variable"
            },
            "parameters": {
               "--privileged": "",
               "--name": "$docker_name"
            }
         }
      }
   }


Splitter
========

The test suite spliter algorithms describe the strategy in which to split up the test suites when there are more than 1 :ref:`iut` & :ref:`execution_space`.

The feature of configuring this is not yet implemented. The idea is to either describe it with JSONTas or as extensions to ETOS.

The default splitter algorithm is round robin.


Dataset
=======

A dataset in `JSONTas <https://jsontas.readthedocs.io>`_ is a data structure with values in it that can be accessed using a '$' notation.
For instance if the dataset contains a dictionary:

.. code-block:: python

   {
      "myname": "Tobias"
   }

Then that value can be accessed using this JSONTas:

.. code-block:: json

   {
      "aname": "$myname"
   }

The dataset structure also has what is called a 'DataStructure' which is a command that can be executed to generate JSON from another source or based on conditions.

Dataset:

.. code-block:: python

   {
      "myname": "Tobias"
   }

JSONTas

.. code-block:: json

   {
      "atitle": {
         "$condition": {
            "if": {
               "key": "$myname",
               "operator": "$eq",
               "value": "Tobias"
            },
            "then": "The best",
            "else": "The worst"
         }
      }
   }

More examples for JSONTas can be found `here <https://jsontas.readthedocs.io/en/latest/examples.html>`_.

There are also several DataStructures implemented into the ETOS environment provider explained below.

json_dumps
----------

Dump a subvalue to string.

JSON
****

.. code-block:: json

   {
      "a_string": {
         "$json_dumps": {
            "a_key": "a_value"
         }
      }
   }


Result
******

.. code-block:: json

   {
      "a_string": "{\"a_key\": \"a_value\"}"
   }


uuid_generate
-------------

Generate a UUID4 value

JSON
****

.. code-block:: json

   {
      "uuid": "$uuid_generate"
   }


Result
******

.. code-block:: json

   {
      "uuid": "a72220c2-eca0-491e-8638-b8e4bdd56f56"
   }


join
----

Join a list of strings together. These strings can be JSON dataset values.

JSON
****

.. code-block:: json

   {
      "joined": {
          "$join": {
              "strings": [
                  "I generated this for you: ",
                  "$uuid_generate"
              ]
          }
      }
   }


Result
******

.. code-block:: json

   {
      "joined": "I generated this for you: 96566362-98e0-47f6-abdb-9e3e45fc7c1a"
   }

API
===

To register a provider into the environment provider you just have to do a POST request to the 'register' API with the JSONTas description.

Example using curl

.. code-block:: bash

   curl -X POST -H "Content-Type: application/json" -d "{\"execution_space_provider\": $(cat myexecutionspaceprovider.json)}" http://environment-provider/register

You can also register multiple providers at once

.. code-block:: bash

   curl -X POST -H "Content-Type: application/json" -d "{\"execution_space_provider\": $(cat myexecutionspaceprovider.json), \"log_are_provider\": $(cat mylogareaprovider.json), \"iut_provider\": $(cat myiutprovider.json)}" http://environment-provider/register

Note that it may take a short while for a provider to be updated.
