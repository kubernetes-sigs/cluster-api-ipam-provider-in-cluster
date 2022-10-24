# Dependency Licenses

{{ range . }}
  - {{.Name}}@{{.Version}} ([{{.LicenseName}}]({{.LicenseURL}}))
{{- end }}
