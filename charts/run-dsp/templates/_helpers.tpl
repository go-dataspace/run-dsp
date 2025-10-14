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
Set up TLS volumes for for remote connections.
*/}}
{{- define "run-dsp.remoteVolumes" -}}
{{- $name := .name -}}
{{- if not .insecure -}}
{{- with .caCert.configMap -}}
- name: {{ $name }}-ca
  configMap:
    name: {{ . | quote }}
{{- end }}
{{- with .clientCertSecret }}
- name: {{ $name }}-client-tls
  secret:
    secretName: {{ . | quote }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Set up TLS volumes for for remote connections.
*/}}
{{- define "run-dsp.remoteVolumeMounts" -}}
{{- $name := .name -}}
{{- if not .insecure -}}
{{- with .caCert.configMap -}}
- name: {{ $name }}-ca
  mountPath: /certs/{{ $name }}/ca
  readOnly: true
{{- end }}
{{- with .clientCertSecret }}
- name: {{ $name }}-client-tls
  mountPath: /certs/{{ $name }}/client
  readOnly: true
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Set up TLS settings for config settions
*/}}
{{- define "run-dsp.tlsconf" -}}
{{- $name := .name -}}
insecure = {{ .insecure }}
{{- if not .insecure }}
{{- if .caCert.configMap }}
caCert = "/certs/{{ $name }}/ca/{{ .caCert.key }}"
{{- end }}
{{- with .clientCertSecret }}
clientCert = "/certs/{{ $name }}/client/tls.crt"
clientCertKey = "/certs/{{ $name }}/client/tls.key"
{{- end }}
{{- end }}
{{- end }}
