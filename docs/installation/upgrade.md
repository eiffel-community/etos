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
# Upgrade

To upgrade ETOS simply upgrade the controller to the latest version. ETOS ships with default, tested, versions of each service and an upgrade of the ETOS controller will make it so that all Cluster resources will automatically deploy the new versions.

So a simple request like this should be sufficient.

```bash
kubectl apply -f https://github.com/eiffel-community/etos/releases/latest/download/install.yaml
```

If you have defined custom versions of services in your Cluster spec, then those services will not be automatically upgraded. To upgrade them manually you'll have to read the latest version of the service in the correct file here `https://github.com/eiffel-community/etos/tree/RELEASETAG/defaults` and update the Cluster specifications.
