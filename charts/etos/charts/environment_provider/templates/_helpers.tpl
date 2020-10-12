{{/*
Expand the name of the chart.
*/}}
{{- define "environment-provider.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "etos.containerImage" -}}from: etos{{ end }}
{{- define "etos.appUrl" -}}from: etos {{ end }}

{{- define "environment-provider.host" -}}
{{- $urlDict := urlParse (.Values.global.environmentProviderUrl) -}}
{{- $host := get $urlDict "host" -}}
{{- printf "%s" $host }}
{{- end }}
 
{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "environment-provider.fullname" -}}
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
{{- define "environment-provider.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "environment-provider.labels" -}}
helm.sh/chart: {{ include "environment-provider.chart" . }}
{{ include "environment-provider.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "environment-provider.selectorLabels" -}}
app.kubernetes.io/name: {{ include "environment-provider.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "environment-provider.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "environment-provider.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
