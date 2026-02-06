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
# Core Concepts

## Determinism

ETOS strives to be as determinstic as possible in regards to how it executes tests. In the test suite file it shall be defined exactly what to run and how to run it and ETOS' job is to execute it exactly according to that specification.
For even greater determinism it is recommended to always have absolute values in the test suite, such as using the commit ID (sha-1) when checkout out tests instead of branches.

While there are ways to bend this determinism a little, for instance by test parallelization, where tests don't always run in the same order unless specified we don't see that as a big problem since we are of the belief that tests should be order-agnostic.
If your tests are not order-agnostic, then they must have a specified dependency between them (not yet implemented).

## Separation of concerns

It is difficult for a test engineer and a system engineer to know how to execute tests that they themselves have not written.
It is not as easy for a test automation engineer to know what tests are relevant at any given time and it is hard for a test engineer and a test automation engineer to know how to set up an orchestration system that fits well into a CI pipeline.

This is where teh separation of concerns in ETOS comes to play. With ETOS we are trying to hide the complexity that you don't need to worry about and let you worry about what you know.
That means that the test engineer shall only need to know what tests to run at a given moment, the test automation engineer shall only need to know how to write tests and how those tests are run and none of them need to know how ETOS is set up and running.

We believe that this will give the best outcome in large, complex, CI systems in which ETOS integrates.
