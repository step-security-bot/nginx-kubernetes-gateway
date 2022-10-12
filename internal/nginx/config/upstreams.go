package config

import (
	"fmt"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
	templates "github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/template"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state"
)

const (
	// nginx502Server is used as a backend for services that cannot be resolved (have no IP address).
	nginx502Server = "unix:/var/lib/nginx/nginx-502-server.sock"
	// invalidBackendRef is used as an upstream name for invalid backend references.
	invalidBackendRef = "invalid-backend-ref"
)

func executeUpstreams(conf state.Configuration) []byte {
	t := templates.NewTemplate([]http.Upstream{})
	upstreams := createUpstreams(conf.Upstreams)

	return t.Execute(upstreams)
}

func createUpstreams(upstreams []state.Upstream) []http.Upstream {
	// capacity is the number of upstreams + 1 for the invalid backend ref upstream
	ups := make([]http.Upstream, 0, len(upstreams)+1)

	for _, u := range upstreams {
		ups = append(ups, createUpstream(u))
	}

	ups = append(ups, createInvalidBackendRefUpstream())

	return ups
}

func createUpstream(up state.Upstream) http.Upstream {
	if len(up.Endpoints) == 0 {
		return http.Upstream{
			Name: up.Name,
			Servers: []http.UpstreamServer{
				{
					Address: nginx502Server,
				},
			},
		}
	}

	upstreamServers := make([]http.UpstreamServer, len(up.Endpoints))
	for idx, ep := range up.Endpoints {
		upstreamServers[idx] = http.UpstreamServer{
			Address: fmt.Sprintf("%s:%d", ep.Address, ep.Port),
		}
	}

	return http.Upstream{
		Name:    up.Name,
		Servers: upstreamServers,
	}
}

func createInvalidBackendRefUpstream() http.Upstream {
	return http.Upstream{
		Name: invalidBackendRef,
		Servers: []http.UpstreamServer{
			{
				Address: nginx502Server,
			},
		},
	}
}
