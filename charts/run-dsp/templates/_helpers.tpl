{{/*
Expand the name of the chart.
*/}}
{{- define "helm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "helm.fullname" -}}
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

{{- define "helm.providerAddress" -}}
{{- if .Values.provider.address -}}
{{- .Values.provider.address -}}
{{- else -}}
{{- printf "%s-%s.%s:%s" .Release.Name "rdsp-s3" .Release.Namespace "9090" -}}
{{- end -}}
{{- end -}}

{{- define "helm.contractServiceAddress" -}}
{{- if .Values.contractService.address -}}
{{- .Values.contractService.address -}}
{{- else -}}
{{- printf "%s-%s.%s:%s" .Release.Name "reference-contract-service" .Release.Namespace "9092" -}}
{{- end -}}
{{- end -}}

{{- define "helm.authnServiceAddress" -}}
{{- if .Values.authnService.address -}}
{{- .Values.authnService.address -}}
{{- else -}}
{{- printf "%s-%s.%s:%s" .Release.Name "reference-authn-service" .Release.Namespace "9093" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "helm.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "helm.labels" -}}
helm.sh/chart: {{ include "helm.chart" . }}
{{ include "helm.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "helm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "helm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "helm.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "helm.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Set up TLS settings for client config sections
*/}}
{{- define "run-dsp.tlsconf" -}}
{{- $name := .name -}}
insecure = {{ .insecure }}
{{- if not .insecure }}
{{- if .caCert.configMap }}
caCert = "/certs/ca/{{ .caCert.key }}"
{{- end }}
clientCert = "/certs/client/tls.crt"
clientCertKey = "/certs/client/tls.key"
{{- end }}
{{- end }}
