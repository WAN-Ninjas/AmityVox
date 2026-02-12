{{/*
Expand the name of the chart.
*/}}
{{- define "amityvox.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "amityvox.fullname" -}}
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
{{- define "amityvox.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "amityvox.labels" -}}
helm.sh/chart: {{ include "amityvox.chart" . }}
{{ include "amityvox.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "amityvox.selectorLabels" -}}
app.kubernetes.io/name: {{ include "amityvox.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "amityvox.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "amityvox.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database URL — use existing secret, explicit config, or derive from subchart.
*/}}
{{- define "amityvox.databaseUrl" -}}
{{- if .Values.config.database.url }}
{{- .Values.config.database.url }}
{{- else if .Values.postgresql.enabled }}
{{- printf "postgres://%s:%s@%s-postgresql:5432/%s?sslmode=disable" .Values.postgresql.auth.username .Values.postgresql.auth.password (include "amityvox.fullname" .) .Values.postgresql.auth.database }}
{{- else }}
{{- fail "Either config.database.url or postgresql.enabled must be set" }}
{{- end }}
{{- end }}

{{/*
NATS URL — use explicit config or derive from subchart.
*/}}
{{- define "amityvox.natsUrl" -}}
{{- if .Values.config.nats.url }}
{{- .Values.config.nats.url }}
{{- else if .Values.nats.enabled }}
{{- printf "nats://%s-nats:4222" (include "amityvox.fullname" .) }}
{{- else }}
{{- fail "Either config.nats.url or nats.enabled must be set" }}
{{- end }}
{{- end }}

{{/*
Cache URL — use explicit config or derive from subchart.
*/}}
{{- define "amityvox.cacheUrl" -}}
{{- if .Values.config.cache.url }}
{{- .Values.config.cache.url }}
{{- else if .Values.redis.enabled }}
{{- printf "redis://%s-dragonfly-master:6379" (include "amityvox.fullname" .) }}
{{- else }}
{{- fail "Either config.cache.url or redis.enabled must be set" }}
{{- end }}
{{- end }}
