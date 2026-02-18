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
# Installation Guide

For a production-like environment, you can install ETOS on a Kubernetes cluster.
While these instructions will give you a serviceable ETOS instance, it is recommended to use a more robust installation method for production environments.
Please read the [production considerations](production.md) for more information on how to install ETOS in a production environment.

### Prerequisites

- Kubernetes cluster

### Installation

####  First deploy the ETOS controller to your Kubernetes cluster:

```bash
kubectl apply -f https://github.com/eiffel-community/etos/releases/latest/download/install.yaml
```

#### To create an ETOS cluster create a Cluster specification file (e.g. `cluster.yaml`) with the following content:

```yaml
apiVersion: etos.eiffel-community.org/v1alpha1
kind: Cluster
metadata:
  name: etos-cluster
spec:
  etos:
    config:
      encryptionKey:
        value: "ZmgcW2Qz43KNJfIuF0vYCoPneViMVyObH4GR8R9JE4g=" # Update this with your own encryption key, you can generate one using `openssl rand -base64 32`
      routingKeyTag: "etos-routing-key"  # RabbitMQ routing key tag to use for this cluster
      etosApiURL: "http://etos-api.etos.svc.cluster.local" # URL to the ETOS API, update this if you have a different setup
  suiteRunner:
    logListener:
      etosQueueName: "*-testlog"  # Queue name for ETOS internal communication
  suiteStarter:
    eiffelQueueName: "etos-suite-starter"  # Queue name for receiving Eiffel events for ETOSv0
  database:
    deploy: true  # Let the cluster deploy the ETCD database for you.
    # host: "https://externally-hosted-etcd" # Hostname for an external ETCD database
    # port: 2379 # Port for an external ETCD database
  messageBus:
    eiffel:
      deploy: true  # Let the cluster deploy the RabbitMQ message bus for you.
      # host: "https://externally-hosted-rabbitmq" # Hostname for an external RabbitMQ message bus
      # exchange: "etos" # Exchange name for the RabbitMQ message bus
      # username: "username" # Username for an external RabbitMQ message bus
      # port: 5672 # Port for an external RabbitMQ message bus
      # ssl: false # Whether to use SSL for an external RabbitMQ message bus
      # vhost: "/" # Virtual host for an external RabbitMQ message bus
      # password:
      #   valueFrom:
      #     secretKeyRef:
      #       name: "rabbitmq-secret"
      #       key: "password"
    etos:
      deploy: true  # Let the cluster deploy the RabbitMQ message bus for you.
      # host: "https://externally-hosted-rabbitmq" # Hostname for an external RabbitMQ message bus
      # exchange: "etos" # Exchange name for the RabbitMQ message bus
      # username: "username" # Username for an external RabbitMQ message bus
      # port: 5672 # Port for an external RabbitMQ message bus
      # ssl: false # Whether to use SSL for an external RabbitMQ message bus
      # vhost: "/" # Virtual host for an external RabbitMQ message bus
      # password:
      #   valueFrom:
      #     secretKeyRef:
      #       name: "rabbitmq-secret"
      #       key: "password"
  eventRepository:
    deploy: true  # Let the cluster deploy the event repository for you.
    eiffelQueueName: "etos-event-repository"  # Queue name for receiving Eiffel events for the event repository
    # host: "https://externally-hosted-graphql-api" # Hostname for an external event repository
    mongo:
      deploy: true  # Let the cluster deploy the MongoDB database for the event repository
      # uri: "mongodb://username:password@mongodb:27017"
```

#### Apply the Cluster specification to your Kubernetes cluster:

```bash
kubectl create namespace etos
kubectl apply -f cluster.yaml -n etos
```

#### Verify the deployment by running tests with ETOS. For more information on how to run tests, please see the [getting started guide](../getting_started/index.md).
