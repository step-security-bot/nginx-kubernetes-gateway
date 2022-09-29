package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state"
)

// nginx502Server is used as a backend for services that cannot be resolved (have no IP address).
const nginx502Server = "unix:/var/lib/nginx/nginx-502-server.sock"

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Generator

// Generator generates NGINX configuration.
type Generator interface {
	// Generate generates NGINX configuration from internal representation.
	Generate(configuration state.Configuration) []byte
}

// GeneratorImpl is an implementation of Generator
type GeneratorImpl struct {
	executor *templateExecutor
}

// NewGeneratorImpl creates a new GeneratorImpl.
func NewGeneratorImpl() *GeneratorImpl {
	return &GeneratorImpl{
		executor: newTemplateExecutor(),
	}
}

func (g *GeneratorImpl) Generate(conf state.Configuration) []byte {
	httpServers := generateHTTPServers(conf)

	httpUpstreams := generateHTTPUpstreams(conf.Upstreams)

	retVal := append(g.executor.ExecuteForHTTPUpstreams(httpUpstreams), g.executor.ExecuteForHTTPServers(httpServers)...)

	return retVal
}

func generateHTTPServers(conf state.Configuration) HTTPServers {
	confServers := append(conf.HTTPServers, conf.SSLServers...)

	servers := make([]Server, 0, len(confServers)+2)

	if len(conf.HTTPServers) > 0 {
		defaultHTTPServer := generateDefaultHTTPServer()

		servers = append(servers, defaultHTTPServer)
	}

	if len(conf.SSLServers) > 0 {
		defaultSSLServer := generateDefaultSSLServer()

		servers = append(servers, defaultSSLServer)
	}

	for _, s := range confServers {
		servers = append(servers, generateServer(s))
	}

	return HTTPServers{Servers: servers}
}

func generateDefaultSSLServer() Server {
	return Server{IsDefaultSSL: true}
}

func generateDefaultHTTPServer() Server {
	return Server{IsDefaultHTTP: true}
}

func generateServer(virtualServer state.VirtualServer) Server {
	s := Server{ServerName: virtualServer.Hostname}

	listenerPort := 80

	if virtualServer.SSL != nil {
		s.SSL = &SSL{
			Certificate:    virtualServer.SSL.CertificatePath,
			CertificateKey: virtualServer.SSL.CertificatePath,
		}

		listenerPort = 443
	}

	if len(virtualServer.PathRules) == 0 {
		// generate default "/" 404 location
		s.Locations = []Location{{Path: "/", Return: &Return{Code: StatusNotFound}}}
		return s
	}

	locs := make([]Location, 0, len(virtualServer.PathRules)) // FIXME(pleshakov): expand with rule.Routes
	for _, rule := range virtualServer.PathRules {
		matches := make([]httpMatch, 0, len(rule.MatchRules))

		for matchRuleIdx, r := range rule.MatchRules {
			m := r.GetMatch()

			var loc Location

			// handle case where the only route is a path-only match
			// generate a standard location block without http_matches.
			if len(rule.MatchRules) == 1 && isPathOnlyMatch(m) {
				loc = Location{
					Path: rule.Path,
				}
				locs = append(locs, Location{
					Path:      rule.Path,
					ProxyPass: generateProxyPass(r.UpstreamName),
				})
			} else {
				path := createPathForMatch(rule.Path, matchRuleIdx)
				loc = generateMatchLocation(path)
				matches = append(matches, createHTTPMatch(m, path))
			}

			// FIXME(pleshakov): There could be a case when the filter has the type set but not the corresponding field.
			// For example, type is v1beta1.HTTPRouteFilterRequestRedirect, but RequestRedirect field is nil.
			// The validation webhook catches that.
			// If it doesn't work as expected, such situation is silently handled below in findFirstFilters.
			// Consider reporting an error. But that should be done in a separate validation layer.

			// RequestRedirect and proxying are mutually exclusive.
			if r.Filters.RequestRedirect != nil {
				loc.Return = generateReturnValForRedirectFilter(r.Filters.RequestRedirect, listenerPort)
			} else {
				loc.ProxyPass = generateProxyPass(r.UpstreamName)
			}

			locs = append(locs, loc)
		}

		if len(matches) > 0 {
			b, err := json.Marshal(matches)
			if err != nil {
				// panic is safe here because we should never fail to marshal the match unless we constructed it incorrectly.
				panic(fmt.Errorf("could not marshal http match: %w", err))
			}

			pathLoc := Location{
				Path:         rule.Path,
				HTTPMatchVar: string(b),
			}

			locs = append(locs, pathLoc)
		}
	}

	s.Locations = locs

	return s
}

func generateReturnValForRedirectFilter(filter *v1beta1.HTTPRequestRedirectFilter, listenerPort int) *Return {
	if filter == nil {
		return nil
	}

	hostname := "$host"
	if filter.Hostname != nil {
		hostname = string(*filter.Hostname)
	}

	// FIXME(pleshakov): Unknown values here must result in the implementation setting the Attached Condition for
	// the Route to  `status: False`, with a Reason of `UnsupportedValue`. In that case, all routes of the Route will be
	// ignored. NGINX will return 500. This should be implemented in the validation layer.
	code := StatusFound
	if filter.StatusCode != nil {
		code = StatusCode(*filter.StatusCode)
	}

	port := listenerPort
	if filter.Port != nil {
		port = int(*filter.Port)
	}

	// FIXME(pleshakov): Same as the FIXME about StatusCode above.
	scheme := "$scheme"
	if filter.Scheme != nil {
		scheme = *filter.Scheme
	}

	return &Return{
		Code: code,
		URL:  fmt.Sprintf("%s://%s:%d$request_uri", scheme, hostname, port),
	}
}

func generateHTTPUpstreams(upstreams []state.Upstream) HTTPUpstreams {
	// capacity is the number of upstreams + 1 for the invalid backend ref upstream
	ups := make([]Upstream, 0, len(upstreams)+1)

	for _, u := range upstreams {
		ups = append(ups, generateUpstream(u))
	}

	ups = append(ups, generateInvalidBackendRefUpstream())

	return HTTPUpstreams{
		Upstreams: ups,
	}
}

func generateUpstream(up state.Upstream) Upstream {
	if len(up.Endpoints) == 0 {
		return Upstream{
			Name: up.Name,
			Servers: []UpstreamServer{
				{
					Address: nginx502Server,
				},
			},
		}
	}

	upstreamServers := make([]UpstreamServer, len(up.Endpoints))
	for idx, ep := range up.Endpoints {
		upstreamServers[idx] = UpstreamServer{
			Address: fmt.Sprintf("%s:%d", ep.Address, ep.Port),
		}
	}

	return Upstream{
		Name:    up.Name,
		Servers: upstreamServers,
	}
}

func generateInvalidBackendRefUpstream() Upstream {
	return Upstream{
		Name: state.InvalidBackendRef,
		Servers: []UpstreamServer{
			{
				Address: nginx502Server,
			},
		},
	}
}

func generateProxyPass(address string) string {
	return "http://" + address
}

func generateMatchLocation(path string) Location {
	return Location{
		Path:     path,
		Internal: true,
	}
}

func createPathForMatch(path string, routeIdx int) string {
	return fmt.Sprintf("%s_route%d", path, routeIdx)
}

// httpMatch is an internal representation of an HTTPRouteMatch.
// This struct is marshaled into a string and stored as a variable in the nginx location block for the route's path.
// The NJS httpmatches module will lookup this variable on the request object and compare the request against the Method, Headers, and QueryParams contained in httpMatch.
// If the request satisfies the httpMatch, the request will be internally redirected to the location RedirectPath by NGINX.
type httpMatch struct {
	// Any represents a match with no match conditions.
	Any bool `json:"any,omitempty"`
	// Method is the HTTPMethod of the HTTPRouteMatch.
	Method v1beta1.HTTPMethod `json:"method,omitempty"`
	// Headers is a list of HTTPHeaders name value pairs with the format "{name}:{value}".
	Headers []string `json:"headers,omitempty"`
	// QueryParams is a list of HTTPQueryParams name value pairs with the format "{name}={value}".
	QueryParams []string `json:"params,omitempty"`
	// RedirectPath is the path to redirect the request to if the request satisfies the match conditions.
	RedirectPath string `json:"redirectPath,omitempty"`
}

func createHTTPMatch(match v1beta1.HTTPRouteMatch, redirectPath string) httpMatch {
	hm := httpMatch{
		RedirectPath: redirectPath,
	}

	if isPathOnlyMatch(match) {
		hm.Any = true
		return hm
	}

	if match.Method != nil {
		hm.Method = *match.Method
	}

	if match.Headers != nil {
		headers := make([]string, 0, len(match.Headers))
		headerNames := make(map[string]struct{})

		// FIXME(kate-osborn): For now we only support type "Exact".
		for _, h := range match.Headers {
			if *h.Type == v1beta1.HeaderMatchExact {
				// duplicate header names are not permitted by the spec
				// only configure the first entry for every header name (case-insensitive)
				lowerName := strings.ToLower(string(h.Name))
				if _, ok := headerNames[lowerName]; !ok {
					headers = append(headers, createHeaderKeyValString(h))
					headerNames[lowerName] = struct{}{}
				}
			}
		}
		hm.Headers = headers
	}

	if match.QueryParams != nil {
		params := make([]string, 0, len(match.QueryParams))

		// FIXME(kate-osborn): For now we only support type "Exact".
		for _, p := range match.QueryParams {
			if *p.Type == v1beta1.QueryParamMatchExact {
				params = append(params, createQueryParamKeyValString(p))
			}
		}
		hm.QueryParams = params
	}

	return hm
}

// The name and values are delimited by "=". A name and value can always be recovered using strings.SplitN(arg,"=", 2).
// Query Parameters are case-sensitive so case is preserved.
func createQueryParamKeyValString(p v1beta1.HTTPQueryParamMatch) string {
	return p.Name + "=" + p.Value
}

// The name and values are delimited by ":". A name and value can always be recovered using strings.Split(arg, ":").
// Header names are case-insensitive while header values are case-sensitive (e.g. foo:bar == FOO:bar, but foo:bar != foo:BAR).
// We preserve the case of the name here because NGINX allows us to lookup the header names in a case-insensitive manner.
func createHeaderKeyValString(h v1beta1.HTTPHeaderMatch) string {
	return string(h.Name) + ":" + h.Value
}

func isPathOnlyMatch(match v1beta1.HTTPRouteMatch) bool {
	return match.Method == nil && match.Headers == nil && match.QueryParams == nil
}
