{{- define "idle-scale.labels" -}}
app.kubernetes.io/name: idle-scale
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: "{{ .Chart.Name }}-{{ .Chart.Version }}"
{{- end -}}

{{- define "idle-scale.controllerName" -}}
{{ .Release.Name }}-idle-scale-controller
{{- end -}}
