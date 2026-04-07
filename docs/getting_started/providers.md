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
# Providers

This guide will help create a custom provider for use within ETOS, if the pre-deployed default providers are not working out.

Providers can be written in any programming language, but the custom providers are written in Golang and we have several interfaces and a package for supporting development of providers in Go so we recommend using this programming language.

A Provider is a piece of software that runs in a docker image, is started by ETOS and creates `LogArea`, `IUT` or `ExecutionSpace` resources in Kubernetes.
The provider shall also implement a way to release the provider if necessary. If ETOS aborts a testrun or if something times out then ETOS will execute the provider to call release on the provided resource.
This is necessary in order to, for example, stop a job from continuing on executing tests.

## The interface

The provider interface is defined [here](https://github.com/eiffel-community/etos/blob/main/pkg/provider/provider.go) and if using Go, can be implemented easily.

```go
type Provider interface {
	Provision(ctx context.Context, logger logr.Logger, cfg ProvisionConfig) error
	Release(ctx context.Context, logger logr.Logger, cfg ReleaseConfig) error
}
```

There are two parts of a provider, Provision and Release.
When ETOS executes the Provision part it will provide these input parameters:

- -namespace - Which Kubernetes namespace the testrun is executing in
- -environment-request - The EnvironmentRequest resource that is requesting
- -provider - The name of your provider (the name of the Provider resource in Kubernetes)

When ETOS executes the Release part it will provide these input parameters:

- -release - boolean, telling the program that this is a release call
- -namespace - Which Kubernetes namespace the testrun is executing in
- -name - The name of the resource that is being released
- -nodelete - An optional boolean input. If set, the provider shall not delete the resource

## Order

The providers are executed in a specific order, every time. This order is important.
When providing an environment ETOS will execute in the order `IUT provider`, `Log area provider` and `Execution space provider`.
When releasing an environment ETOS will execute in the order `Execution space provider`, `Log area provider` and `IUT provider`.

With the fact that ETOS can run tests in parallel it means that the `Log area provider` and `Execution space provider` need to read the previous providers resources in order to know how many IUTs were created in order to provide enough resources.

If you implement your provider using Go and use the package provided in [ETOS](https://github.com/eiffel-community/etos/blob/main/pkg/provider/provider.go) then this is done for you already, if using any other programming language or if you are not using the provider package from ETOS, then you'll need to request the IUT resource from Kubernetes. They are labeled with the EnvironmentRequest ID for easier querying.
Example:
```go
// GetIUTs fetches all IUTs for an environmentrequest from Kubernetes.
func GetIUTs(ctx context.Context, environmentRequestID, namespace string) (v1alpha2.IutList, error) {
	var iuts v1alpha2.IutList
	cli, err := KubernetesClient()
	if err != nil {
		return iuts, err
	}
	err = cli.List(
		ctx,
		&iuts,
		client.InNamespace(namespace),
		client.MatchingLabels{"etos.eiffel-community.github.io/environment-request-id": environmentRequestID},
	)
	return iuts, err
}
```

## Example code

- [Execution space provider](https://github.com/eiffel-community/etos/blob/main/cmd/executionspaceprovider/main.go)
- [Log area provider](https://github.com/eiffel-community/etos/blob/main/cmd/logareaprovider/main.go)
- [IUT provider](https://github.com/eiffel-community/etos/blob/main/cmd/iutprovider/main.go)
