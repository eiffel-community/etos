{{/*
Expand the name of the chart.
*/}}
{{- define "etos.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "etos.appUrl" -}}
{{- printf "%s.%s" (include "etos.name" . | replace "_" "-") (.Values.global.baseClusterUrl) }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "etos.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "etos.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "etos.labels" -}}
helm.sh/chart: {{ include "etos.chart" . }}
{{ include "etos.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "etos.selectorLabels" -}}
app.kubernetes.io/name: {{ include "etos.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "etos.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "etos.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "etos.namespace.value" -}}
{{- printf "%s" (include "etos.namespace" . | replace "\nnamespace: " "") }}
{{- end }}

//{{- define "etos.namespace" -}}
//{{- if eq .Values.global.namespace ""}}
//namespace: {{ include "etos.name" . }}
//{{- else }}
//namespace: {{ .Values.global.namespace }}
//{{- end }}
//{{- end }}

{{- define "etos.rabbitMQSecrets" -}}
password: {{ printf "%s" .Values.rabbitMQ.password | b64enc }}
username: {{ printf "%s" .Values.rabbitMQ.username | b64enc }}
{{- end }}

{{- define "etos.redisSecret" -}}
password: {{ printf "%s" .Values.redis.password | b64enc }}
{{- end }}

{{- define "etos.suiteRunnerContainerImage" -}}
{{ if .Values.global.development }}
SUITE_RUNNER: {{ .Values.suiteRunnerContainerImage }}:dev
{{- else }}
SUITE_RUNNER: {{ printf "%s:%s" .Values.suiteRunnerContainerImage .Values.suiteRunnerContainerImageTag }}
{{- end }}
{{- end }}

{{- define "etos.containerImage" -}}
{{- if .Values.global.development }}
image: {{ .Values.image.dev.repository }}:dev
imagePullPolicy: Always
{{- else }}
image: {{ .Values.image.prod.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end }}
{{- end }}
