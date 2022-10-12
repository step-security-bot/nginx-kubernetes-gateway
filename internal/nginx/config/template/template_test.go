package template_test

import (
	"testing"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
	templates "github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/template"
)

func TestNewTemplatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewTemplate() did not panic")
		}
	}()

	// NewTemplate should panic if resourceType is not supported.
	_ = templates.NewTemplate("not supported")
}

func TestNewTemplate(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewTemplate() panicked with %v", r)
		}
	}()

	resourceTypes := []interface{}{
		[]http.Server{},
		[]http.SplitClient{},
		[]http.Upstream{},
	}

	for _, rt := range resourceTypes {
		_ = templates.NewTemplate(rt)
	}
}

func TestTemplate_Execute(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("template.Execute() panicked with %v", r)
		}
	}()

	tmpl := templates.NewTemplate([]http.Server{})
	bytes := tmpl.Execute([]http.Server{})
	if len(bytes) == 0 {
		t.Error("template.Execute() did not generate anything")
	}
}

func TestTemplate_ExecutePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("template.Execute() did not panic")
		}
	}()

	tmpl := templates.NewTemplate([]http.Server{})
	_ = tmpl.Execute("not-correct-data")
}
