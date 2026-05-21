{{/*
Common helpers for the vfx chart.
*/}}

{{- define "vfx.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "vfx.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "vfx.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "vfx.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/name: {{ include "vfx.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "vfx.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{ printf "%s:%s" .Values.image.repository $tag }}
{{- end -}}

{{- define "vfx.jwtSecretName" -}}
{{- if .Values.existingJwtSecretName -}}
{{ .Values.existingJwtSecretName }}
{{- else -}}
{{ include "vfx.fullname" . }}-jwt
{{- end -}}
{{- end -}}

{{- define "vfx.gatewayServiceAccountName" -}}
{{ include "vfx.fullname" . }}-gateway
{{- end -}}
