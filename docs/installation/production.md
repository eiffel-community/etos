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
# Production considerations

While ETOS can install all services that are required for ETOS to run, they should not be considered production-ready deployments.

For a production environment it is recommended that you deploy these services separately for ETOS to use.
The services that are required for ETOS to run are:

- RabbitMQ message bus for communication between ETOS components and for sending and receiving Eiffel events. [RabbitMQ](https://www.rabbitmq.com/)
- Event repository for storing and querying Eiffel events [Eiffel GraphQL API](https://github.com/eiffel-community/eiffel-graphql-api)
    - A database to support the event repository.

The ETCD database that ETOS deploys can be used in production, but it may struggle with higher workloads. For a production environment, it is recommended to deploy it separately and configure it to handle the expected workload.
