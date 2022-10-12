package state

import (
	"fmt"
	"sort"

	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state/resolver"
)

const wildcardHostname = "~^"

// Configuration is an internal representation of Gateway configuration.
// We can think of Configuration as an intermediate state between the Gateway API resources and the data plane (NGINX)
// configuration.
type Configuration struct {
	// HTTPServers holds all HTTPServers.
	// FIXME(pleshakov) We assume that all servers are HTTP and listen on port 80.
	HTTPServers []VirtualServer
	// SSLServers holds all SSLServers.
	// FIXME(kate-osborn) We assume that all SSL servers listen on port 443.
	SSLServers []VirtualServer
	// Upstreams holds all Upstreams.
	Upstreams []Upstream
	// BackendGroups holds all BackendGroups.
	BackendGroups []BackendGroup
}

// VirtualServer is a virtual server.
type VirtualServer struct {
	// Hostname is the hostname of the server.
	Hostname string
	// PathRules is a collection of routing rules.
	PathRules []PathRule
	// SSL holds the SSL configuration options fo the server.
	SSL *SSL
}

type Upstream struct {
	// Name is the name of the Upstream. Will be unique for each service/port combination.
	Name string
	// Endpoints are the endpoints of the Upstream.
	Endpoints []resolver.Endpoint
}

type SSL struct {
	// CertificatePath is the path to the certificate file.
	CertificatePath string
}

// PathRule represents routing rules that share a common path.
type PathRule struct {
	// Path is a path. For example, '/hello'.
	Path string
	// MatchRules holds routing rules.
	MatchRules []MatchRule
}

// Filters hold the filters for a MatchRule.
type Filters struct {
	RequestRedirect *v1beta1.HTTPRequestRedirectFilter
}

// MatchRule represents a routing rule. It corresponds directly to a Match in the HTTPRoute resource.
// An HTTPRoute is guaranteed to have at least one rule with one match.
// If no rule or match is specified by the user, the default rule {{path:{ type: "PathPrefix", value: "/"}}} is set by the schema.
type MatchRule struct {
	// MatchIdx is the index of the rule in the Rule.Matches.
	MatchIdx int
	// RuleIdx is the index of the corresponding rule in the HTTPRoute.
	RuleIdx int
	// Filters holds the filters for the MatchRule.
	Filters Filters
	// BackendGroup is the group of Backends that the rule routes to.
	BackendGroup BackendGroup
	// Source is the corresponding HTTPRoute resource.
	// FIXME(pleshakov): Consider referencing only the parts needed for the config generation rather than
	// the entire resource.
	Source *v1beta1.HTTPRoute
}

// GetMatch returns the HTTPRouteMatch of the Route .
func (r *MatchRule) GetMatch() v1beta1.HTTPRouteMatch {
	return r.Source.Spec.Rules[r.RuleIdx].Matches[r.MatchIdx]
}

// buildConfiguration builds the Configuration from the graph.
// FIXME(pleshakov) For now we only handle paths with prefix matches. Handle exact and regex matches
func buildConfiguration(graph *graph) Configuration {
	if graph.GatewayClass == nil || !graph.GatewayClass.Valid {
		return Configuration{}
	}

	if graph.Gateway == nil {
		return Configuration{}
	}

	upstreams := buildUpstreams(graph.Gateway.Listeners)
	httpServers, sslServers := buildServers(graph.Gateway.Listeners)
	backendGroups := buildBackendGroups(graph.Gateway.Listeners)

	config := Configuration{
		HTTPServers:   httpServers,
		SSLServers:    sslServers,
		Upstreams:     upstreams,
		BackendGroups: backendGroups,
	}

	return config
}

func buildBackendGroups(listeners map[string]*listener) []BackendGroup {
	// There can be duplicate backend groups if a route is attached to multiple listeners.
	// We use a map to deduplicate them.
	uniqueGroups := make(map[string]BackendGroup, 0)

	for _, l := range listeners {

		if !l.Valid {
			continue
		}

		for _, r := range l.Routes {
			for _, group := range r.BackendRefs.ByRule {
				if _, ok := uniqueGroups[group.GroupName()]; !ok {
					uniqueGroups[group.GroupName()] = group
				}
			}
		}

	}

	groups := make([]BackendGroup, 0, len(uniqueGroups))
	for _, group := range uniqueGroups {
		groups = append(groups, group)
	}

	// sort upstreams for test-ability
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].GroupName() < groups[j].GroupName()
	})

	return groups
}

func buildServers(listeners map[string]*listener) (http, ssl []VirtualServer) {
	rulesForProtocol := map[v1beta1.ProtocolType]*hostPathRules{
		v1beta1.HTTPProtocolType:  newHostPathRules(),
		v1beta1.HTTPSProtocolType: newHostPathRules(),
	}

	for _, l := range listeners {
		if l.Valid {
			rules := rulesForProtocol[l.Source.Protocol]
			rules.upsertListener(l)
		}
	}

	httpRules := rulesForProtocol[v1beta1.HTTPProtocolType]
	sslRules := rulesForProtocol[v1beta1.HTTPSProtocolType]

	return httpRules.buildServers(), sslRules.buildServers()
}

type hostPathRules struct {
	rulesPerHost     map[string]map[string]PathRule
	listenersForHost map[string]*listener
	listeners        []*listener
}

func newHostPathRules() *hostPathRules {
	return &hostPathRules{
		rulesPerHost:     make(map[string]map[string]PathRule),
		listenersForHost: make(map[string]*listener),
		listeners:        make([]*listener, 0),
	}
}

func (hpr *hostPathRules) upsertListener(l *listener) {
	if l.Source.Protocol == v1beta1.HTTPSProtocolType {
		hpr.listeners = append(hpr.listeners, l)
	}

	for _, r := range l.Routes {
		var hostnames []string

		for _, h := range r.Source.Spec.Hostnames {
			if _, exist := l.AcceptedHostnames[string(h)]; exist {
				hostnames = append(hostnames, string(h))
			}
		}

		for _, h := range hostnames {
			hpr.listenersForHost[h] = l

			if _, exist := hpr.rulesPerHost[h]; !exist {
				hpr.rulesPerHost[h] = make(map[string]PathRule)
			}
		}

		for i, rule := range r.Source.Spec.Rules {
			filters := createFilters(rule.Filters)

			for _, h := range hostnames {
				for j, m := range rule.Matches {
					path := getPath(m.Path)

					rule, exist := hpr.rulesPerHost[h][path]
					if !exist {
						rule.Path = path
					}

					rule.MatchRules = append(rule.MatchRules, MatchRule{
						MatchIdx:     j,
						RuleIdx:      i,
						Source:       r.Source,
						BackendGroup: r.BackendRefs.ByRule[ruleIndex(i)],
						Filters:      filters,
					})

					hpr.rulesPerHost[h][path] = rule
				}
			}
		}
	}
}

func (hpr *hostPathRules) buildServers() []VirtualServer {
	servers := make([]VirtualServer, 0, len(hpr.rulesPerHost)+len(hpr.listeners))

	for h, rules := range hpr.rulesPerHost {
		s := VirtualServer{
			Hostname:  h,
			PathRules: make([]PathRule, 0, len(rules)),
		}

		l, ok := hpr.listenersForHost[h]
		if !ok {
			panic(fmt.Sprintf("no listener found for hostname: %s", h))
		}

		if l.SecretPath != "" {
			s.SSL = &SSL{CertificatePath: l.SecretPath}
		}

		for _, r := range rules {
			sortMatchRules(r.MatchRules)

			s.PathRules = append(s.PathRules, r)
		}

		// sort rules for predictable order
		sort.Slice(s.PathRules, func(i, j int) bool {
			return s.PathRules[i].Path < s.PathRules[j].Path
		})

		servers = append(servers, s)
	}

	for _, l := range hpr.listeners {
		hostname := getListenerHostname(l.Source.Hostname)
		// generate a 404 ssl server block for listeners with no routes or listeners with wildcard (match-all) routes
		// FIXME(kate-osborn): when we support regex hostnames (e.g. *.example.com) we will have to modify this check to catch regex hostnames.
		if len(l.Routes) == 0 || hostname == wildcardHostname {
			s := VirtualServer{
				Hostname: hostname,
			}

			if l.SecretPath != "" {
				s.SSL = &SSL{CertificatePath: l.SecretPath}
			}

			servers = append(servers, s)
		}
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Hostname < servers[j].Hostname
	})

	return servers
}

func buildUpstreams(listeners map[string]*listener) []Upstream {
	// There can be duplicate upstreams if multiple routes reference the same upstream.
	// We use a map to deduplicate them.
	uniqueUpstreams := make(map[string]Upstream)

	for _, l := range listeners {

		if !l.Valid {
			continue
		}

		for _, route := range l.Routes {
			for name, eps := range route.BackendRefs.Resolved {
				if _, ok := uniqueUpstreams[name]; !ok {
					uniqueUpstreams[name] = Upstream{
						Name:      name,
						Endpoints: eps,
					}
				}
			}
		}
	}

	upstreams := make([]Upstream, 0, len(uniqueUpstreams))
	for _, u := range uniqueUpstreams {
		upstreams = append(upstreams, u)
	}

	// sort upstreams for test-ability
	sort.Slice(upstreams, func(i, j int) bool {
		return upstreams[i].Name < upstreams[j].Name
	})

	return upstreams
}

func getListenerHostname(h *v1beta1.Hostname) string {
	name := getHostname(h)
	if name == "" {
		return wildcardHostname
	}

	return name
}

func getPath(path *v1beta1.HTTPPathMatch) string {
	if path == nil || path.Value == nil || *path.Value == "" {
		return "/"
	}
	return *path.Value
}

func createFilters(filters []v1beta1.HTTPRouteFilter) Filters {
	var result Filters

	for _, f := range filters {
		switch f.Type {
		case v1beta1.HTTPRouteFilterRequestRedirect:
			result.RequestRedirect = f.RequestRedirect
			// using the first filter
			return result
		}
	}

	return result
}
