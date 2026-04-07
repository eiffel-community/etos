<!---
   Copyright Axis Communications AB
   For a full list of individual contributors, please see the commit history.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
--->

<img src="images/etos-logo.png" alt="ETOS" width="350"/>

ETOS (Eiffel Test Orchestration System) is a new test orchestration system which takes away control of how to run and what to run from the system itself and places it into the hands of the relevant engineers.

The idea of having a system dictate what and how to run is finished. Let's bring back control to the testers.

With ETOS we define how to run tests using recipes and we define what to run with collections of recipes.

| [Test Engineer](roles/test_engineer.md) | [Test Automation Engineer](roles/test_automation_engineer.md) | [System Engineer](roles/system_engineer.md) |
| ------------- | ------------- | ------------- |
| Create collections | Create recipes | Deploy ETOS |
| Analyze results | Create tests | Infrastructure |
| What to run? | How to run? | Where to run? |

This means that only people who knows how and what to run decide these factors. ETOS will only receive the collection of recipes and execute it accordingly.
You can also mix test suites. For instance, let's say you want to run a `go` unittest and a `python` function test in the same suite, that's easy to do; just add them to your collection.

This is the strength of ETOS. Relying on the humans to decide what to run, how to run and where to run.

ETOS is a collection of multiple services working together, all bound together by a Kubernetes controller.

[Documentation](https://etos.readthedocs.io)

## Features

### Generic test suite execution based solely on JSON.

A recipe is a JSON file describing how to run a test suite. It contains all the information needed to run the test suite, such as the testrunner to use, the parameters to pass to the testrunner, and any other information needed to run the test suite.

### Mix and match test suites, regardless of programming language.

ETOS does not care about the programming language of the test suite. It only cares about the recipe, which is a JSON file describing how to run the test suite. This means that you can run any test suite, as long as you have a recipe for it and a testrunner that can execute it.

### Separation of concerns between testers, test automation engineers and system engineers.

ETOS separates the concerns of testers, test automation engineers and system engineers. 
Testers are responsible for creating collections of recipes, test automation engineers are responsible for creating the recipes and tests, and system engineers are responsible for deploying ETOS and maintaining the infrastructure.

### Eiffel protocol implementation.

ETOS implements the Eiffel protocol, which is a protocol for technology agnostic machine-to-machine communication in continuous integration and delivery pipelines, aimed at securing scalability, flexibility and traceability. Eiffel is based on the concept of decentralized real time messaging, both to drive the continuous integration and delivery system and to document it.

## Getting Started

### Running tests with ETOS
Target audience: test engineers and test automation engineers.

[Getting started guide](getting_started/index.md)

### Installing ETOS
Target audience: system engineers.

[Installing ETOS](installation/index.md)
