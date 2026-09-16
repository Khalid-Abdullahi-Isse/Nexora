{{- define "social.kong.config" -}}
_format_version: "3.0"
_transform: true
services:
{{- range $name, $paths := .Values.kong.routes }}
{{- $svc := index $.Values.services $name }}
{{- if $svc.enabled }}
  - name: {{ $name }}-service
    host: {{ $name }}-service
    port: {{ $svc.port }}
    protocol: http
    connect_timeout: {{ $.Values.kong.connectTimeout }}
    read_timeout: {{ $.Values.kong.readTimeout }}
    write_timeout: {{ $.Values.kong.writeTimeout }}
    retries: 0
    routes:
      - name: {{ $name }}-api
        paths:
        {{- range $paths }}
          - {{ printf "~%s(?:/|$)" . | quote }}
        {{- end }}
        strip_path: false
        preserve_host: false
      {{- if eq $name "auth" }}
      - name: auth-sensitive
        paths:
          - '~/api/v1/auth/(?:login|register|refresh)/?$'
        methods: [POST]
        regex_priority: 20
        strip_path: false
        plugins:
          - name: rate-limiting
            config:
              minute: {{ $.Values.kong.rateLimit.authMinute }}
              policy: local
              limit_by: ip
              hide_client_headers: true
      {{- end }}
  - name: {{ $name }}-health
    host: {{ $name }}-service
    port: {{ $svc.port }}
    protocol: http
    path: /health
    connect_timeout: {{ $.Values.kong.connectTimeout }}
    read_timeout: {{ $.Values.kong.readTimeout }}
    write_timeout: {{ $.Values.kong.writeTimeout }}
    retries: 0
    routes:
      - name: {{ $name }}-health
        paths:
          - {{ printf "~%s$" (index $.Values.kong.healthPaths $name) | quote }}
          {{- if eq $name "auth" }}
          - '~/health$'
          {{- end }}
        methods: [GET, OPTIONS]
        regex_priority: 100
        strip_path: true
{{- end }}
{{- end }}
plugins:
  - name: cors
    config:
      origins:
        {{- range splitList "," .Values.config.ALLOWED_ORIGINS }}
        - {{ trim . | quote }}
        {{- end }}
      methods: [GET, POST, PATCH, PUT, DELETE, OPTIONS, HEAD]
      headers: [Authorization, Content-Type, X-CSRF-Token, X-CSRF-Protection, X-Request-ID]
      exposed_headers: [X-Request-ID, Retry-After, RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset]
      credentials: true
      max_age: 600
      preflight_continue: false
  - name: rate-limiting
    config:
      minute: {{ .Values.kong.rateLimit.minute }}
      policy: local
      limit_by: ip
      hide_client_headers: true
  - name: request-size-limiting
    config:
      allowed_payload_size: {{ .Values.kong.maxBodyKilobytes }}
      size_unit: kilobytes
      require_content_length: false
  - name: correlation-id
    config:
      header_name: X-Request-ID
      generator: uuid
      echo_downstream: true
{{- end -}}
