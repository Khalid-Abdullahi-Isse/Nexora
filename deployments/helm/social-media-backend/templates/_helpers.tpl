{{- define "social.name" -}}
{{- printf "%s-backend" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- define "social.labels" -}}
app.kubernetes.io/name: social-media-backend
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}
{{- define "social.sa" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "social.name" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}
{{- define "social.trustMounts" -}}
{{- if .Values.postgres.tls.enabled }}
- name: postgres-ca
  mountPath: /trust/postgres
  readOnly: true
{{- end }}
{{- if .Values.redis.tls.enabled }}
- name: redis-ca
  mountPath: /trust/redis
  readOnly: true
{{- end }}
{{- end -}}
{{- define "social.trustVolumes" -}}
{{- if .Values.postgres.tls.enabled }}
- name: postgres-ca
  secret:
    secretName: {{ .Values.postgres.tls.existingSecret }}
    items:
      - key: ca.crt
        path: ca.crt
{{- end }}
{{- if .Values.redis.tls.enabled }}
- name: redis-ca
  secret:
    secretName: {{ .Values.redis.tls.existingSecret }}
    items:
      - key: ca.crt
        path: ca.crt
{{- end }}
{{- end -}}
{{- define "social.deployment" -}}
{{- $root := .root -}}
{{- $name := .name -}}
{{- $svc := index $root.Values.services $name -}}
{{- $image := index $root.Values.images $name -}}
{{- if $svc.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $name }}-service
  namespace: {{ $root.Release.Namespace }}
  labels:
    {{- include "social.labels" $root | nindent 4 }}
  annotations:
    argocd.argoproj.io/sync-wave: "0"
spec:
  replicas: {{ $svc.replicas }}
  selector:
    matchLabels:
      app.kubernetes.io/instance: {{ $root.Release.Name }}
      app.kubernetes.io/component: {{ $name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/instance: {{ $root.Release.Name }}
        app.kubernetes.io/component: {{ $name }}
      annotations:
        checksum/config: {{ include (print $root.Template.BasePath "/configmap.yaml") $root | sha256sum }}
        checksum/secrets: {{ include (print $root.Template.BasePath "/secret.yaml") $root | sha256sum }}
        deployment/rollout-version: {{ $root.Values.rolloutVersion | quote }}
    spec:
      serviceAccountName: {{ include "social.sa" $root }}
      automountServiceAccountToken: false
      terminationGracePeriodSeconds: {{ $root.Values.terminationGracePeriodSeconds }}
      securityContext:
        {{- toYaml $root.Values.podSecurityContext | nindent 8 }}
      {{- with $root.Values.global.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      initContainers:
        - name: wait-dependencies
          image: {{ $root.Values.postgres.image | quote }}
          securityContext:
            {{- toYaml $root.Values.containerSecurityContext | nindent 12 }}
          command: [sh, -ec]
          args:
            - 'until pg_isready -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" && nc -z "$REDIS_HOST" "$REDIS_PORT"; do sleep 2; done'
          env:
            - name: PGHOST
              value: {{ $root.Values.postgres.host | quote }}
            - name: PGPORT
              value: {{ $root.Values.postgres.port | quote }}
            - name: PGUSER
              value: {{ $svc.databaseUser | quote }}
            - name: PGDATABASE
              value: {{ $root.Values.postgres.database | quote }}
            - name: REDIS_HOST
              value: {{ $root.Values.redis.host | quote }}
            - name: REDIS_PORT
              value: {{ $root.Values.redis.port | quote }}
          resources:
            {{- toYaml $root.Values.migration.resources | nindent 12 }}
      containers:
        - name: {{ $name }}
          image: {{ printf "%s:%s" $image.repository $image.tag | quote }}
          imagePullPolicy: {{ $root.Values.global.imagePullPolicy }}
          securityContext:
            {{- toYaml $root.Values.containerSecurityContext | nindent 12 }}
          ports:
            - name: http
              containerPort: {{ $svc.port }}
          envFrom:
            - configMapRef:
                name: {{ include "social.name" $root }}-config
          env:
            - name: {{ upper $name }}_SERVICE_PORT
              value: {{ $svc.port | quote }}
            - name: POSTGRES_USER
              value: {{ $svc.databaseUser | quote }}
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: {{ $root.Values.secrets.existingSecret }}
                  key: {{ $svc.passwordKey }}
            - name: REDIS_ADDR
              valueFrom:
                secretKeyRef:
                  name: {{ $root.Values.secrets.existingSecret }}
                  key: REDIS_ADDR
          volumeMounts:
            - name: keys
              mountPath: /keys
              readOnly: true
            - name: tmp
              mountPath: /tmp
            {{- include "social.trustMounts" $root | nindent 12 }}
          {{- range $kind, $settings := $root.Values.probes }}
          {{ $kind }}Probe:
            httpGet:
              path: {{ if and (eq $name "post") (eq $kind "readiness") }}/ready{{ else }}/health{{ end }}
              port: http
            {{- toYaml $settings | nindent 12 }}
          {{- end }}
          {{- if $root.Values.resources.enabled }}
          resources:
            {{- toYaml (mergeOverwrite (deepCopy $root.Values.resources.defaults) $svc.resources) | nindent 12 }}
          {{- end }}
      volumes:
        - name: tmp
          emptyDir: {}
        - name: keys
          secret:
            secretName: {{ $root.Values.jwt.existingSecret }}
            defaultMode: 0440
            items:
              - key: public-keys.json
                path: public-keys.json
              {{- if eq $name "auth" }}
              - key: private.pem
                path: private.pem
              {{- end }}
        {{- include "social.trustVolumes" $root | nindent 8 }}
{{- end }}
{{- end -}}
{{- define "social.service" -}}
{{- $svc := index .root.Values.services .name -}}
{{- if $svc.enabled }}
apiVersion: v1
kind: Service
metadata:
  name: {{ .name }}-service
  namespace: {{ .root.Release.Namespace }}
  labels:
    {{- include "social.labels" .root | nindent 4 }}
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/instance: {{ .root.Release.Name }}
    app.kubernetes.io/component: {{ .name }}
  ports:
    - name: http
      port: {{ $svc.port }}
      targetPort: http
{{- end }}
{{- end -}}
