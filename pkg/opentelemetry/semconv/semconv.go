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

// ETOSProviderEnvironmentRequest returns an attribute.String with the key
// "etos.provider.environment_request" and the provided value.
func ETOSProviderEnvironmentRequest(val string) attribute.KeyValue {
	return attribute.String("etos.provider.environment_request", val)
}

// ETOSProviderNamespace returns an attribute.String with the key
// "etos.provider.namespace" and the provided value.
func ETOSProviderNamespace(val string) attribute.KeyValue {
	return attribute.String("etos.provider.namespace", val)
}

// ETOSProviderName returns an attribute.String with the key
// "etos.provider.name" and the provided value.
func ETOSProviderName(val string) attribute.KeyValue {
	return attribute.String("etos.provider.name", val)
}

// ETOSProviderLogArea returns an attribute.String with the key
// "etos.provider.log_area" and the provided value.
func ETOSProviderLogArea(val string) attribute.KeyValue {
	return attribute.String("etos.provider.log_area", val)
}

// ETOSProviderAmount returns an attribute.Int with the key
// "etos.provider.amount" and the provided value.
func ETOSProviderAmount(val int) attribute.KeyValue {
	return attribute.Int("etos.provider.amount", val)
}

// ETOSProviderMinimumAmount returns an attribute.Int with the key
// "etos.provider.minimum_amount" and the provided value.
func ETOSProviderMinimumAmount(val int) attribute.KeyValue {
	return attribute.Int("etos.provider.minimum_amount", val)
}

// ETOSProviderMaximumAmount returns an attribute.Int with the key
// "etos.provider.maximum_amount" and the provided value.
func ETOSProviderMaximumAmount(val int) attribute.KeyValue {
	return attribute.Int("etos.provider.maximum_amount", val)
}

// ETOSProviderConclusion returns an attribute.String with the key
// "etos.provider.conclusion" and the provided value.
func ETOSProviderConclusion(val string) attribute.KeyValue {
	return attribute.String("etos.provider.conclusion", val)
}

// ETOSProviderVerdict returns an attribute.String with the key
// "etos.provider.verdict" and the provided value.
func ETOSProviderVerdict(val string) attribute.KeyValue {
	return attribute.String("etos.provider.verdict", val)
}

// ETOSProviderDescription returns an attribute.String with the key
// "etos.provider.description" and the provided value.
func ETOSProviderDescription(val string) attribute.KeyValue {
	return attribute.String("etos.provider.description", val)
}

// ETOSLogAreaProviderLiveLogs returns an attribute.String with the key
// "etos.provider.log_area.live_logs" and the provided value.
func ETOSLogAreaProviderLiveLogs(val string) attribute.KeyValue {
	return attribute.String("etos.provider.log_area.live_logs", val)
}

// ETOSLogAreaProviderLogAreaUploadURL returns an attribute.String with the key
// "etos.provider.log_area.upload.url" and the provided value.
func ETOSLogAreaProviderLogAreaUploadURL(val string) attribute.KeyValue {
	return attribute.String("etos.provider.log_area.upload.url", val)
}
