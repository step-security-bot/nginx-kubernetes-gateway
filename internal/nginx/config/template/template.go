package template

import (
	"bytes"
	"fmt"
	gotemplate "text/template"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
)

var (
	serversTemplate      = gotemplate.Must(gotemplate.New("servers").Parse(serversTemplateText))
	splitClientsTemplate = gotemplate.Must(gotemplate.New("split_clients").Parse(splitClientsTemplateText))
	upstreamsTemplate    = gotemplate.Must(gotemplate.New("upstreams").Parse(upstreamsTemplateText))
)

// Template is a wrapper around the text/template package.
type Template struct {
	source *gotemplate.Template
}

// NewTemplate creates a new Template for the given resource type.
// Panics if the resource type is not supported.
func NewTemplate(resourceType interface{}) Template {
	switch resourceType.(type) {
	case []http.Server:
		return Template{source: serversTemplate}
	case []http.SplitClient:
		return Template{source: splitClientsTemplate}
	case []http.Upstream:
		return Template{source: upstreamsTemplate}
	default:
		panic(fmt.Sprintf("unknown resource type: %T", resourceType))
	}
}

// Execute executes the template with the given data.
func (t Template) Execute(data interface{}) []byte {
	var buf bytes.Buffer

	err := t.source.Execute(&buf, data)
	if err != nil {
		panic(err)
	}

	return buf.Bytes()
}
