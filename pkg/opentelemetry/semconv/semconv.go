// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package semconv

import "go.opentelemetry.io/otel/attribute"

func ETOSProviderEnvironmentRequest(val string) attribute.KeyValue {
	return attribute.String("etos.provider.environment_request", val)
}

func ETOSProviderNamespace(val string) attribute.KeyValue {
	return attribute.String("etos.provider.namespace", val)
}

func ETOSProviderName(val string) attribute.KeyValue {
	return attribute.String("etos.provider.name", val)
}

func ETOSProviderLogArea(val string) attribute.KeyValue {
	return attribute.String("etos.provider.log_area", val)
}

func ETOSProviderAmount(val int) attribute.KeyValue {
	return attribute.Int("etos.provider.amount", val)
}

func ETOSProviderMinimumAmount(val int) attribute.KeyValue {
	return attribute.Int("etos.provider.minimum_amount", val)
}

func ETOSProviderMaximumAmount(val int) attribute.KeyValue {
	return attribute.Int("etos.provider.maximum_amount", val)
}

func ETOSProviderConclusion(val string) attribute.KeyValue {
	return attribute.String("etos.provider.conclusion", val)
}

func ETOSProviderVerdict(val string) attribute.KeyValue {
	return attribute.String("etos.provider.verdict", val)
}

func ETOSProviderDescription(val string) attribute.KeyValue {
	return attribute.String("etos.provider.description", val)
}

func ETOSLogAreaProviderLiveLogs(val string) attribute.KeyValue {
	return attribute.String("etos.provider.log_area.live_logs", val)
}

func ETOSLogAreaProviderLogAreaUploadURL(val string) attribute.KeyValue {
	return attribute.String("etos.provider.log_area.upload.url", val)
}
