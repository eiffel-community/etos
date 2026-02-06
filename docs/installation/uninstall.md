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

# Uninstall

To uninstall ETOS, cleanly, you should first clean up any testruns, environments and environment requests in the Kubernetes cluster

```bash
kubectl delete environments,environmentrequests,testruns --all
```

After that delete all ETOS Clusters

```bash
kubectl delete clusters --all
```

And then we can delete the ETOS controller.

```bash
kubectl delete -f https://github.com/eiffel-community/etos/releases/latest/download/install.yaml
```

After that ETOS should be fully uninstalled from your Kubernetes cluster.
