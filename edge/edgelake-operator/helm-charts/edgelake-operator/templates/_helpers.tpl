{{/*
Expand the name of the chart.
*/}}
{{- define "edgelake-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "edgelake-operator.fullname" -}}
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
{{- define "edgelake-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "edgelake-operator.labels" -}}
helm.sh/chart: {{ include "edgelake-operator.chart" . }}
{{ include "edgelake-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "edgelake-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "edgelake-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: {{ .Values.metadata.app_name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "edgelake-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "edgelake-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the node name - use metadata.app_name if NODE_NAME is not set
*/}}
{{- define "edgelake-operator.nodeName" -}}
{{- if .Values.node_configs.general.NODE_NAME }}
{{- .Values.node_configs.general.NODE_NAME }}
{{- else }}
{{- .Values.metadata.app_name }}
{{- end }}
{{- end }}
