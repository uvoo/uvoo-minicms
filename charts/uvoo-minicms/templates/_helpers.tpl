{{- define "uvoo-minicms.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "uvoo-minicms.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-minicms.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "uvoo-minicms.labels" -}}
helm.sh/chart: {{ include "uvoo-minicms.chart" . }}
{{ include "uvoo-minicms.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "uvoo-minicms.selectorLabels" -}}
app.kubernetes.io/name: {{ include "uvoo-minicms.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "uvoo-minicms.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "uvoo-minicms.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "uvoo-minicms.adminSecretName" -}}
{{- default (printf "%s-admin" (include "uvoo-minicms.fullname" .)) .Values.admin.existingSecret -}}
{{- end -}}

{{- define "uvoo-minicms.writerServiceName" -}}
{{- printf "%s-writer" (include "uvoo-minicms.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "uvoo-minicms.readerDeploymentName" -}}
{{- printf "%s-readers" (include "uvoo-minicms.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "uvoo-minicms.tlsSecretName" -}}
{{- default (printf "%s-tls" (include "uvoo-minicms.fullname" .)) .Values.ingress.tls.secretName -}}
{{- end -}}

{{- define "uvoo-minicms.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{- define "uvoo-minicms.trustProxyHeaders" -}}
{{- if eq (toString .Values.config.trustProxyHeaders) "" -}}
{{- ternary "true" "false" .Values.ingress.enabled -}}
{{- else -}}
{{- .Values.config.trustProxyHeaders | toString -}}
{{- end -}}
{{- end -}}
