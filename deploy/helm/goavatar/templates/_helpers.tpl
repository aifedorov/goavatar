{{- define "goavatar.namespace" -}}
{{- .Values.namespace.name -}}
{{- end -}}

{{- define "goavatar.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag }}
{{- end -}}

{{- define "goavatar.serviceAccountName" -}}
{{- .Values.serviceAccount.name -}}
{{- end -}}
