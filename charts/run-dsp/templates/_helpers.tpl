{{/*
Expand the name of the chart.
*/}}
{{- define "run-dsp.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "run-dsp.fullname" -}}
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

{{- define "run-dsp.providerAddress" -}}
{{- if .Values.provider.address -}}
{{- .Values.provider.address -}}
{{- else -}}
{{- printf "%s-%s.%s:%s" .Release.Name "rdsp-s3" .Release.Namespace "9090" -}}
{{- end -}}
{{- end -}}

{{- define "run-dsp.contractServiceAddress" -}}
{{- if .Values.contractService.address -}}
{{- .Values.contractService.address -}}
{{- else -}}
{{- printf "%s-%s.%s:%s" .Release.Name "reference-contract-service" .Release.Namespace "9092" -}}
{{- end -}}
{{- end -}}

{{- define "run-dsp.authnServiceAddress" -}}
{{- if .Values.authnService.address -}}
{{- .Values.authnService.address -}}
{{- else -}}
{{- printf "%s-%s.%s:%s" .Release.Name "reference-authn-service" .Release.Namespace "9093" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "run-dsp.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "run-dsp.labels" -}}
helm.sh/chart: {{ include "run-dsp.chart" . }}
{{ include "run-dsp.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "run-dsp.selectorLabels" -}}
app.kubernetes.io/name: {{ include "run-dsp.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "run-dsp.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "run-dsp.fullname" .) .Values.serviceAccount.name }}
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
