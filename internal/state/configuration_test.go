package state

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/helpers"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state/resolver"
)

func TestBuildConfiguration(t *testing.T) {
	createRoute := func(name string, hostname string, listenerName string, paths ...string) *v1beta1.HTTPRoute {
		rules := make([]v1beta1.HTTPRouteRule, 0, len(paths))
		for _, p := range paths {
			rules = append(rules, v1beta1.HTTPRouteRule{
				Matches: []v1beta1.HTTPRouteMatch{
					{
						Path: &v1beta1.HTTPPathMatch{
							Value: helpers.GetStringPointer(p),
						},
					},
				},
			})
		}
		return &v1beta1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test",
				Name:      name,
			},
			Spec: v1beta1.HTTPRouteSpec{
				CommonRouteSpec: v1beta1.CommonRouteSpec{
					ParentRefs: []v1beta1.ParentReference{
						{
							Namespace:   (*v1beta1.Namespace)(helpers.GetStringPointer("test")),
							Name:        "gateway",
							SectionName: (*v1beta1.SectionName)(helpers.GetStringPointer(listenerName)),
						},
					},
				},
				Hostnames: []v1beta1.Hostname{
					v1beta1.Hostname(hostname),
				},
				Rules: rules,
			},
		}
	}

	addFilters := func(hr *v1beta1.HTTPRoute, filters []v1beta1.HTTPRouteFilter) *v1beta1.HTTPRoute {
		for i := range hr.Spec.Rules {
			hr.Spec.Rules[i].Filters = filters
		}
		return hr
	}

	fooUpstreamName := "test_foo_80"

	fooUpstream := Upstream{
		Name: fooUpstreamName,
		Endpoints: []resolver.Endpoint{
			{
				Address: "10.0.0.0",
				Port:    8080,
			},
		},
	}

	createBackendGroup := func(nsname types.NamespacedName, idx int) BackendGroup {
		return BackendGroup{
			Source:  nsname,
			RuleIdx: idx,
			Backends: []BackendRef{
				{
					Name:   fooUpstreamName,
					Valid:  true,
					Weight: 1,
				},
			},
		}
	}

	createInternalRoute := func(source *v1beta1.HTTPRoute, validSectionName string, groups ...BackendGroup) *route {
		r := &route{
			Source:                 source,
			InvalidSectionNameRefs: make(map[string]struct{}),
			ValidSectionNameRefs:   map[string]struct{}{validSectionName: {}},
			BackendRefs: BackendRefs{
				Resolved: map[string][]resolver.Endpoint{
					"test_foo_80": {
						{
							Address: "10.0.0.0",
							Port:    8080,
						},
					},
				},
				ByRule: make(map[ruleIndex]BackendGroup),
			},
		}
		for idx, group := range groups {
			r.BackendRefs.ByRule[ruleIndex(idx)] = group
		}

		return r
	}

	createTestResources := func(name, hostname, listenerName string, paths ...string) (
		*v1beta1.HTTPRoute, []BackendGroup, *route,
	) {
		hr := createRoute(name, hostname, listenerName, paths...)
		groups := make([]BackendGroup, 0, len(paths))
		for idx := range paths {
			groups = append(groups, createBackendGroup(types.NamespacedName{Namespace: "test", Name: name}, idx))
		}

		route := createInternalRoute(hr, listenerName, groups...)
		return hr, groups, route
	}

	hr1, hr1Groups, routeHR1 := createTestResources("hr-1", "foo.example.com", "listener-80-1", "/")
	hr2, hr2Groups, routeHR2 := createTestResources("hr-2", "bar.example.com", "listener-80-1", "/")
	hr3, hr3Groups, routeHR3 := createTestResources("hr-3", "foo.example.com", "listener-80-1", "/", "/third")
	hr4, hr4Groups, routeHR4 := createTestResources("hr-4", "foo.example.com", "listener-80-1", "/fourth", "/")

	httpsHR1, httpsHR1Groups, httpsRouteHR1 := createTestResources(
		"https-hr-1",
		"foo.example.com",
		"listener-443-1",
		"/",
	)

	httpsHR2, httpsHR2Groups, httpsRouteHR2 := createTestResources(
		"https-hr-2",
		"bar.example.com",
		"listener-443-1",
		"/",
	)

	httpsHR3, httpsHR3Groups, httpsRouteHR3 := createTestResources(
		"https-hr-3",
		"foo.example.com",
		"listener-443-1",
		"/", "/third",
	)

	httpsHR4, httpsHR4Groups, httpsRouteHR4 := createTestResources(
		"https-hr-4",
		"foo.example.com",
		"listener-443-1",
		"/fourth", "/",
	)

	httpsHR5 := createRoute("https-hr-5", "example.com", "listener-443-with-hostname", "/")
	httpsHR5Group := createBackendGroup(types.NamespacedName{Namespace: httpsHR5.Namespace, Name: httpsHR5.Name}, 0)
	httpsHR5Group.Backends[0].Valid = false

	httpsRouteHR5 := &route{
		Source: httpsHR5,
		ValidSectionNameRefs: map[string]struct{}{
			"listener-443-with-hostname": {},
		},
		InvalidSectionNameRefs: map[string]struct{}{},
		BackendRefs: BackendRefs{
			ByRule: map[ruleIndex]BackendGroup{
				0: httpsHR5Group,
			},
		},
	}

	redirect := v1beta1.HTTPRouteFilter{
		Type: v1beta1.HTTPRouteFilterRequestRedirect,
		RequestRedirect: &v1beta1.HTTPRequestRedirectFilter{
			Hostname: (*v1beta1.PreciseHostname)(helpers.GetStringPointer("foo.example.com")),
		},
	}

	hr5 := addFilters(
		createRoute("hr-5", "foo.example.com", "listener-80-1", "/"),
		[]v1beta1.HTTPRouteFilter{redirect},
	)

	routeHR5 := &route{
		Source:                 hr5,
		InvalidSectionNameRefs: make(map[string]struct{}),
		ValidSectionNameRefs:   map[string]struct{}{"listener-80-1": {}},
		BackendRefs:            BackendRefs{},
	}

	listener80 := v1beta1.Listener{
		Name:     "listener-80-1",
		Hostname: nil,
		Port:     80,
		Protocol: v1beta1.HTTPProtocolType,
	}

	listener443 := v1beta1.Listener{
		Name:     "listener-443-1",
		Hostname: nil,
		Port:     443,
		Protocol: v1beta1.HTTPSProtocolType,
		TLS: &v1beta1.GatewayTLSConfig{
			Mode: helpers.GetTLSModePointer(v1beta1.TLSModeTerminate),
			CertificateRefs: []v1beta1.SecretObjectReference{
				{
					Kind:      (*v1beta1.Kind)(helpers.GetStringPointer("Secret")),
					Name:      "secret",
					Namespace: (*v1beta1.Namespace)(helpers.GetStringPointer("test")),
				},
			},
		},
	}
	hostname := v1beta1.Hostname("example.com")

	listener443WithHostname := v1beta1.Listener{
		Name:     "listener-443-with-hostname",
		Hostname: &hostname,
		Port:     443,
		Protocol: v1beta1.HTTPSProtocolType,
		TLS: &v1beta1.GatewayTLSConfig{
			Mode: helpers.GetTLSModePointer(v1beta1.TLSModeTerminate),
			CertificateRefs: []v1beta1.SecretObjectReference{
				{
					Kind:      (*v1beta1.Kind)(helpers.GetStringPointer("Secret")),
					Name:      "secret",
					Namespace: (*v1beta1.Namespace)(helpers.GetStringPointer("test")),
				},
			},
		},
	}

	invalidListener := v1beta1.Listener{
		Name:     "invalid-listener",
		Hostname: nil,
		Port:     443,
		Protocol: v1beta1.HTTPSProtocolType,
		TLS:      nil, // missing TLS config
	}

	// nolint:gosec
	secretPath := "/etc/nginx/secrets/secret"

	tests := []struct {
		graph    *graph
		expected Configuration
		msg      string
	}{
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source:    &v1beta1.Gateway{},
					Listeners: map[string]*listener{},
				},
				Routes: map[types.NamespacedName]*route{},
			},
			expected: Configuration{
				HTTPServers:   []VirtualServer{},
				SSLServers:    []VirtualServer{},
				Upstreams:     []Upstream{},
				BackendGroups: []BackendGroup{},
			},
			msg: "no listeners and routes",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-80-1": {
							Source:            listener80,
							Valid:             true,
							Routes:            map[types.NamespacedName]*route{},
							AcceptedHostnames: map[string]struct{}{},
						},
					},
				},
				Routes: map[types.NamespacedName]*route{},
			},
			expected: Configuration{
				HTTPServers:   []VirtualServer{},
				SSLServers:    []VirtualServer{},
				Upstreams:     []Upstream{},
				BackendGroups: []BackendGroup{},
			},
			msg: "http listener with no routes",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-443-1": {
							Source:            listener443, // nil hostname
							Valid:             true,
							Routes:            map[types.NamespacedName]*route{},
							AcceptedHostnames: map[string]struct{}{},
							SecretPath:        secretPath,
						},
						"listener-443-with-hostname": {
							Source:            listener443WithHostname, // non-nil hostname
							Valid:             true,
							Routes:            map[types.NamespacedName]*route{},
							AcceptedHostnames: map[string]struct{}{},
							SecretPath:        secretPath,
						},
					},
				},
				Routes: map[types.NamespacedName]*route{},
			},
			expected: Configuration{
				HTTPServers: []VirtualServer{},
				SSLServers: []VirtualServer{
					{
						Hostname: string(hostname),
						SSL:      &SSL{CertificatePath: secretPath},
					},
					{
						Hostname: wildcardHostname,
						SSL:      &SSL{CertificatePath: secretPath},
					},
				},
				Upstreams:     []Upstream{},
				BackendGroups: []BackendGroup{},
			},
			msg: "https listeners with no routes",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"invalid-listener": {
							Source: invalidListener,
							Valid:  false,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "https-hr-1"}: httpsRouteHR1,
								{Namespace: "test", Name: "https-hr-2"}: httpsRouteHR2,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
								"bar.example.com": {},
							},
							SecretPath: "",
						},
					},
				},
				Routes: map[types.NamespacedName]*route{
					{Namespace: "test", Name: "https-hr-1"}: httpsRouteHR1,
					{Namespace: "test", Name: "https-hr-2"}: httpsRouteHR2,
				},
			},
			expected: Configuration{
				HTTPServers:   []VirtualServer{},
				SSLServers:    []VirtualServer{},
				Upstreams:     []Upstream{},
				BackendGroups: []BackendGroup{},
			},
			msg: "invalid listener",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-80-1": {
							Source: listener80,
							Valid:  true,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "hr-1"}: routeHR1,
								{Namespace: "test", Name: "hr-2"}: routeHR2,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
								"bar.example.com": {},
							},
						},
					},
				},
				Routes: map[types.NamespacedName]*route{
					{Namespace: "test", Name: "hr-1"}: routeHR1,
					{Namespace: "test", Name: "hr-2"}: routeHR2,
				},
			},
			expected: Configuration{
				HTTPServers: []VirtualServer{
					{
						Hostname: "bar.example.com",
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: hr2Groups[0],
										Source:       hr2,
									},
								},
							},
						},
					},
					{
						Hostname: "foo.example.com",
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: hr1Groups[0],
										Source:       hr1,
									},
								},
							},
						},
					},
				},
				SSLServers:    []VirtualServer{},
				Upstreams:     []Upstream{fooUpstream},
				BackendGroups: []BackendGroup{hr1Groups[0], hr2Groups[0]},
			},
			msg: "one http listener with two routes for different hostnames",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-443-1": {
							Source:     listener443,
							Valid:      true,
							SecretPath: secretPath,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "https-hr-1"}: httpsRouteHR1,
								{Namespace: "test", Name: "https-hr-2"}: httpsRouteHR2,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
								"bar.example.com": {},
							},
						},
						"listener-443-with-hostname": {
							Source:     listener443WithHostname,
							Valid:      true,
							SecretPath: secretPath,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "https-hr-5"}: httpsRouteHR5,
							},
							AcceptedHostnames: map[string]struct{}{
								"example.com": {},
							},
						},
					},
				},
				Routes: map[types.NamespacedName]*route{
					{Namespace: "test", Name: "https-hr-1"}: httpsRouteHR1,
					{Namespace: "test", Name: "https-hr-2"}: httpsRouteHR2,
					{Namespace: "test", Name: "https-hr-5"}: httpsRouteHR5,
				},
			},
			expected: Configuration{
				HTTPServers: []VirtualServer{},
				SSLServers: []VirtualServer{
					{
						Hostname: "bar.example.com",
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: httpsHR2Groups[0],
										Source:       httpsHR2,
									},
								},
							},
						},
						SSL: &SSL{
							CertificatePath: secretPath,
						},
					},
					{
						Hostname: "example.com",
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: httpsHR5Group,
										Source:       httpsHR5,
									},
								},
							},
						},
						SSL: &SSL{
							CertificatePath: secretPath,
						},
					},
					{
						Hostname: "foo.example.com",
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: httpsHR1Groups[0],
										Source:       httpsHR1,
									},
								},
							},
						},
						SSL: &SSL{
							CertificatePath: secretPath,
						},
					},
					{
						Hostname: wildcardHostname,
						SSL:      &SSL{CertificatePath: secretPath},
					},
				},
				Upstreams:     []Upstream{fooUpstream},
				BackendGroups: []BackendGroup{httpsHR1Groups[0], httpsHR2Groups[0], httpsHR5Group},
			},
			msg: "two https listeners each with routes for different hostnames",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-80-1": {
							Source: listener80,
							Valid:  true,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "hr-3"}: routeHR3,
								{Namespace: "test", Name: "hr-4"}: routeHR4,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
							},
						},
						"listener-443-1": {
							Source:     listener443,
							Valid:      true,
							SecretPath: secretPath,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "https-hr-3"}: httpsRouteHR3,
								{Namespace: "test", Name: "https-hr-4"}: httpsRouteHR4,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
							},
						},
					},
				},
				Routes: map[types.NamespacedName]*route{
					{Namespace: "test", Name: "hr-3"}:       routeHR3,
					{Namespace: "test", Name: "hr-4"}:       routeHR4,
					{Namespace: "test", Name: "https-hr-3"}: httpsRouteHR3,
					{Namespace: "test", Name: "https-hr-4"}: httpsRouteHR4,
				},
			},
			expected: Configuration{
				HTTPServers: []VirtualServer{
					{
						Hostname: "foo.example.com",
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: hr3Groups[0],
										Source:       hr3,
									},
									{
										MatchIdx:     0,
										RuleIdx:      1,
										BackendGroup: hr4Groups[1],
										Source:       hr4,
									},
								},
							},
							{
								Path: "/fourth",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: hr4Groups[0],
										Source:       hr4,
									},
								},
							},
							{
								Path: "/third",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      1,
										BackendGroup: hr3Groups[1],
										Source:       hr3,
									},
								},
							},
						},
					},
				},
				SSLServers: []VirtualServer{
					{
						Hostname: "foo.example.com",
						SSL: &SSL{
							CertificatePath: secretPath,
						},
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: httpsHR3Groups[0],
										Source:       httpsHR3,
									},
									{
										MatchIdx:     0,
										RuleIdx:      1,
										BackendGroup: httpsHR4Groups[1],
										Source:       httpsHR4,
									},
								},
							},
							{
								Path: "/fourth",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										BackendGroup: httpsHR4Groups[0],
										Source:       httpsHR4,
									},
								},
							},
							{
								Path: "/third",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      1,
										BackendGroup: httpsHR3Groups[1],
										Source:       httpsHR3,
									},
								},
							},
						},
					},
					{
						Hostname: wildcardHostname,
						SSL:      &SSL{CertificatePath: secretPath},
					},
				},
				Upstreams:     []Upstream{fooUpstream},
				BackendGroups: []BackendGroup{hr3Groups[0], hr3Groups[1], hr4Groups[0], hr4Groups[1], httpsHR3Groups[0], httpsHR3Groups[1], httpsHR4Groups[0], httpsHR4Groups[1]},
			},
			msg: "one http and one https listener with two routes with the same hostname with and without collisions",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source:   &v1beta1.GatewayClass{},
					Valid:    false,
					ErrorMsg: "error",
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-80-1": {
							Source: listener80,
							Valid:  true,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "hr-1"}: routeHR1,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
							},
						},
					},
				},
				Routes: map[types.NamespacedName]*route{
					{Namespace: "test", Name: "hr-1"}: routeHR1,
				},
			},
			expected: Configuration{},
			msg:      "invalid gatewayclass",
		},
		{
			graph: &graph{
				GatewayClass: nil,
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-80-1": {
							Source: listener80,
							Valid:  true,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "hr-1"}: routeHR1,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
							},
						},
					},
				},
				Routes: map[types.NamespacedName]*route{
					{Namespace: "test", Name: "hr-1"}: routeHR1,
				},
			},
			expected: Configuration{},
			msg:      "missing gatewayclass",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: nil,
				Routes:  map[types.NamespacedName]*route{},
			},
			expected: Configuration{},
			msg:      "missing gateway",
		},
		{
			graph: &graph{
				GatewayClass: &gatewayClass{
					Source: &v1beta1.GatewayClass{},
					Valid:  true,
				},
				Gateway: &gateway{
					Source: &v1beta1.Gateway{},
					Listeners: map[string]*listener{
						"listener-80-1": {
							Source: listener80,
							Valid:  true,
							Routes: map[types.NamespacedName]*route{
								{Namespace: "test", Name: "hr-5"}: routeHR5,
							},
							AcceptedHostnames: map[string]struct{}{
								"foo.example.com": {},
							},
						},
					},
				},
				Routes: map[types.NamespacedName]*route{
					{Namespace: "test", Name: "hr-5"}: routeHR5,
				},
			},
			expected: Configuration{
				HTTPServers: []VirtualServer{
					{
						Hostname: "foo.example.com",
						PathRules: []PathRule{
							{
								Path: "/",
								MatchRules: []MatchRule{
									{
										MatchIdx:     0,
										RuleIdx:      0,
										Source:       hr5,
										BackendGroup: BackendGroup{},
										Filters: Filters{
											RequestRedirect: redirect.RequestRedirect,
										},
									},
								},
							},
						},
					},
				},
				SSLServers:    []VirtualServer{},
				Upstreams:     []Upstream{},
				BackendGroups: []BackendGroup{},
			},
			msg: "one http listener with one route with filters",
		},
	}

	for _, test := range tests {
		result := buildConfiguration(test.graph)
		if diff := cmp.Diff(test.expected, result); diff != "" {
			t.Errorf("buildConfiguration() %q mismatch (-want +got):\n%s", test.msg, diff)
		}
	}
}

func TestGetPath(t *testing.T) {
	tests := []struct {
		path     *v1beta1.HTTPPathMatch
		expected string
		msg      string
	}{
		{
			path:     &v1beta1.HTTPPathMatch{Value: helpers.GetStringPointer("/abc")},
			expected: "/abc",
			msg:      "normal case",
		},
		{
			path:     nil,
			expected: "/",
			msg:      "nil path",
		},
		{
			path:     &v1beta1.HTTPPathMatch{Value: nil},
			expected: "/",
			msg:      "nil value",
		},
		{
			path:     &v1beta1.HTTPPathMatch{Value: helpers.GetStringPointer("")},
			expected: "/",
			msg:      "empty value",
		},
	}

	for _, test := range tests {
		result := getPath(test.path)
		if result != test.expected {
			t.Errorf("getPath() returned %q but expected %q for the case of %q", result, test.expected, test.msg)
		}
	}
}

func TestCreateFilters(t *testing.T) {
	redirect1 := v1beta1.HTTPRouteFilter{
		Type: v1beta1.HTTPRouteFilterRequestRedirect,
		RequestRedirect: &v1beta1.HTTPRequestRedirectFilter{
			Hostname: (*v1beta1.PreciseHostname)(helpers.GetStringPointer("foo.example.com")),
		},
	}
	redirect2 := v1beta1.HTTPRouteFilter{
		Type: v1beta1.HTTPRouteFilterRequestRedirect,
		RequestRedirect: &v1beta1.HTTPRequestRedirectFilter{
			Hostname: (*v1beta1.PreciseHostname)(helpers.GetStringPointer("bar.example.com")),
		},
	}

	tests := []struct {
		filters  []v1beta1.HTTPRouteFilter
		expected Filters
		msg      string
	}{
		{
			filters:  []v1beta1.HTTPRouteFilter{},
			expected: Filters{},
			msg:      "no filters",
		},
		{
			filters: []v1beta1.HTTPRouteFilter{
				redirect1,
			},
			expected: Filters{
				RequestRedirect: redirect1.RequestRedirect,
			},
			msg: "one filter",
		},
		{
			filters: []v1beta1.HTTPRouteFilter{
				redirect1,
				redirect2,
			},
			expected: Filters{
				RequestRedirect: redirect1.RequestRedirect,
			},
			msg: "two filters, first wins",
		},
	}

	for _, test := range tests {
		result := createFilters(test.filters)
		if diff := cmp.Diff(test.expected, result); diff != "" {
			t.Errorf("createFilters() %q mismatch (-want +got):\n%s", test.msg, diff)
		}
	}
}

func TestMatchRuleGetMatch(t *testing.T) {
	hr := &v1beta1.HTTPRoute{
		Spec: v1beta1.HTTPRouteSpec{
			Rules: []v1beta1.HTTPRouteRule{
				{
					Matches: []v1beta1.HTTPRouteMatch{
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/path-1"),
							},
						},
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/path-2"),
							},
						},
					},
				},
				{
					Matches: []v1beta1.HTTPRouteMatch{
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/path-3"),
							},
						},
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/path-4"),
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name,
		expPath string
		rule MatchRule
	}{
		{
			name:    "first match in first rule",
			expPath: "/path-1",
			rule:    MatchRule{MatchIdx: 0, RuleIdx: 0, Source: hr},
		},
		{
			name:    "second match in first rule",
			expPath: "/path-2",
			rule:    MatchRule{MatchIdx: 1, RuleIdx: 0, Source: hr},
		},
		{
			name:    "second match in second rule",
			expPath: "/path-4",
			rule:    MatchRule{MatchIdx: 1, RuleIdx: 1, Source: hr},
		},
	}

	for _, tc := range tests {
		actual := tc.rule.GetMatch()
		if *actual.Path.Value != tc.expPath {
			t.Errorf("MatchRule.GetMatch() returned incorrect match with path: %s, expected path: %s for test case: %q", *actual.Path.Value, tc.expPath, tc.name)
		}
	}
}

func TestGetListenerHostname(t *testing.T) {
	var emptyHostname v1beta1.Hostname
	var hostname v1beta1.Hostname = "example.com"

	tests := []struct {
		hostname *v1beta1.Hostname
		expected string
		msg      string
	}{
		{
			hostname: nil,
			expected: wildcardHostname,
			msg:      "nil hostname",
		},
		{
			hostname: &emptyHostname,
			expected: wildcardHostname,
			msg:      "empty hostname",
		},
		{
			hostname: &hostname,
			expected: string(hostname),
			msg:      "normal hostname",
		},
	}

	for _, test := range tests {
		result := getListenerHostname(test.hostname)
		if result != test.expected {
			t.Errorf("getListenerHostname() returned %q but expected %q for the case of %q", result, test.expected, test.msg)
		}
	}
}

func TestBuildUpstreams(t *testing.T) {
	fooEndpoints := []resolver.Endpoint{
		{
			Address: "10.0.0.0",
			Port:    8080,
		},
		{
			Address: "10.0.0.1",
			Port:    8080,
		},
		{
			Address: "10.0.0.2",
			Port:    8080,
		},
	}

	barEndpoints := []resolver.Endpoint{
		{
			Address: "11.0.0.0",
			Port:    80,
		},
		{
			Address: "11.0.0.1",
			Port:    80,
		},
		{
			Address: "11.0.0.2",
			Port:    80,
		},
		{
			Address: "11.0.0.3",
			Port:    80,
		},
	}

	bazEndpoints := []resolver.Endpoint{
		{
			Address: "12.0.0.0",
			Port:    80,
		},
	}

	baz2Endpoints := []resolver.Endpoint{
		{
			Address: "13.0.0.0",
			Port:    80,
		},
	}

	routes := map[types.NamespacedName]*route{
		{Name: "hr1", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				Resolved: map[string][]resolver.Endpoint{
					"foo": fooEndpoints,
					"bar": barEndpoints,
				},
			},
		},
		{Name: "hr2", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				Resolved: map[string][]resolver.Endpoint{
					"foo": fooEndpoints, // shouldn't duplicate foo upstream
					"baz": bazEndpoints,
				},
			},
		},
		{Name: "hr3", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				Resolved: map[string][]resolver.Endpoint{
					"nil-endpoints":   nil,
					"empty-endpoints": {},
				},
			},
		},
	}

	routes2 := map[types.NamespacedName]*route{
		{Name: "hr4", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				Resolved: map[string][]resolver.Endpoint{
					"baz":  bazEndpoints, // shouldn't duplicate baz upstream
					"baz2": baz2Endpoints,
				},
			},
		},
	}

	invalidRoutes := map[types.NamespacedName]*route{
		{Name: "invalid", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				Resolved: map[string][]resolver.Endpoint{
					"invalid-endpoint": {
						{
							Address: "invalid",
							Port:    80,
						},
					},
				},
			},
		},
	}

	listeners := map[string]*listener{
		"invalid-listener": {
			Valid:  false,
			Routes: invalidRoutes,
		},
		"listener-1": {
			Valid:  true,
			Routes: routes,
		},
		"listener-2": {
			Valid:  true,
			Routes: routes2,
		},
	}

	expUpstreams := []Upstream{
		{Name: "bar", Endpoints: barEndpoints},
		{Name: "baz", Endpoints: bazEndpoints},
		{Name: "baz2", Endpoints: baz2Endpoints},
		{Name: "empty-endpoints", Endpoints: []resolver.Endpoint{}},
		{Name: "foo", Endpoints: fooEndpoints},
		{Name: "nil-endpoints", Endpoints: nil},
	}

	upstreams := buildUpstreams(listeners)

	if diff := helpers.Diff(expUpstreams, upstreams); diff != "" {
		t.Errorf("buildUpstreams() mismatch: %+v", diff)
	}
}

func TestBuildBackendGroups(t *testing.T) {
	createBackendGroup := func(name string, ruleIdx int, backendNames ...string) BackendGroup {
		backends := make([]BackendRef, len(backendNames))
		for i, name := range backendNames {
			backends[i] = BackendRef{Name: name}
		}

		return BackendGroup{
			Source:   types.NamespacedName{Namespace: "test", Name: name},
			RuleIdx:  ruleIdx,
			Backends: backends,
		}
	}

	hr1Rule0 := createBackendGroup("hr1", 0, "foo", "bar")

	hr1Rule1 := createBackendGroup("hr1", 1, "foo")

	hr2Rule0 := createBackendGroup("hr2", 0, "foo", "bar")

	hr2Rule1 := createBackendGroup("hr2", 1, "foo")

	hr3Rule0 := createBackendGroup("hr3", 0, "foo", "bar")

	hr3Rule1 := createBackendGroup("hr3", 1, "foo")

	hrInvalid := createBackendGroup("hr-invalid", 0, "invalid")

	invalidRoutes := map[types.NamespacedName]*route{
		{Name: "invalid", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				ByRule: map[ruleIndex]BackendGroup{
					0: hrInvalid,
				},
			},
		},
	}

	routes := map[types.NamespacedName]*route{
		{Name: "hr1", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				ByRule: map[ruleIndex]BackendGroup{
					0: hr1Rule0,
					1: hr1Rule1,
				},
			},
		},
		{Name: "hr2", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				ByRule: map[ruleIndex]BackendGroup{
					0: hr2Rule0,
					1: hr2Rule1,
				},
			},
		},
	}

	routes2 := map[types.NamespacedName]*route{
		// this backend group is a dupe and should be ignored.
		{Name: "hr1", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				ByRule: map[ruleIndex]BackendGroup{
					0: hr1Rule0,
					1: hr1Rule1,
				},
			},
		},
		{Name: "hr3", Namespace: "test"}: {
			BackendRefs: BackendRefs{
				ByRule: map[ruleIndex]BackendGroup{
					0: hr3Rule0,
					1: hr3Rule1,
				},
			},
		},
	}

	listeners := map[string]*listener{
		"invalid-listener": {
			Valid:  false,
			Routes: invalidRoutes,
		},
		"listener-1": {
			Valid:  true,
			Routes: routes,
		},
		"listener-2": {
			Valid:  true,
			Routes: routes2,
		},
	}

	expGroups := []BackendGroup{
		hr1Rule0,
		hr1Rule1,
		hr2Rule0,
		hr2Rule1,
		hr3Rule0,
		hr3Rule1,
	}

	result := buildBackendGroups(listeners)

	if diff := helpers.Diff(expGroups, result); diff != "" {
		t.Errorf("buildBackendGroups() mismatch: %+v", diff)
	}
}
