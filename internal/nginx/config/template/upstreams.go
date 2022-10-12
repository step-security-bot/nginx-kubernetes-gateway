package template

// FIXME(kate-osborn): Add upstream zone size for each upstream. This should be dynamically calculated based on the number of upstreams.
var upstreamsTemplateText = `
{{ range $u := . }}
upstream {{ $u.Name }} {
    random two least_conn;
    {{ range $server := $u.Servers }} 
    server {{ $server.Address }};
    {{ end }}
}
{{ end }}`
