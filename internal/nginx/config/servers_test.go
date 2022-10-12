package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/helpers"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state"
)

func TestExecuteServers(t *testing.T) {
	conf := state.Configuration{
		HTTPServers: []state.VirtualServer{
			{
				Hostname: "example.com",
			},
			{
				Hostname: "cafe.example.com",
			},
		},
		SSLServers: []state.VirtualServer{
			{
				Hostname: "example.com",
				SSL: &state.SSL{
					CertificatePath: "cert-path",
				},
			},
			{
				Hostname: "cafe.example.com",
				SSL: &state.SSL{
					CertificatePath: "cert-path",
				},
			},
		},
	}

	expSubStrings := map[string]int{
		"listen 80 default_server;":      1,
		"listen 443 ssl;":                2,
		"listen 443 ssl default_server;": 1,
		"server_name example.com;":       2,
		"server_name cafe.example.com;":  2,
		"ssl_certificate cert-path;":     2,
		"ssl_certificate_key cert-path;": 2,
	}

	servers := string(executeServers(conf))
	for expSubStr, expCount := range expSubStrings {
		if expCount != strings.Count(servers, expSubStr) {
			t.Errorf(
				"executeServers() did not generate servers with substring %q %d times. Servers: %v",
				expSubStr,
				expCount,
				servers,
			)
		}
	}
}

func TestExecuteForDefaultServers(t *testing.T) {
	testcases := []struct {
		conf        state.Configuration
		httpDefault bool
		sslDefault  bool
		msg         string
	}{
		{
			conf:        state.Configuration{},
			httpDefault: false,
			sslDefault:  false,
			msg:         "no servers",
		},
		{
			conf: state.Configuration{
				HTTPServers: []state.VirtualServer{
					{
						Hostname: "example.com",
					},
				},
			},
			httpDefault: true,
			sslDefault:  false,
			msg:         "only HTTP servers",
		},
		{
			conf: state.Configuration{
				SSLServers: []state.VirtualServer{
					{
						Hostname: "example.com",
					},
				},
			},
			httpDefault: false,
			sslDefault:  true,
			msg:         "only HTTPS servers",
		},
		{
			conf: state.Configuration{
				HTTPServers: []state.VirtualServer{
					{
						Hostname: "example.com",
					},
				},
				SSLServers: []state.VirtualServer{
					{
						Hostname: "example.com",
					},
				},
			},
			httpDefault: true,
			sslDefault:  true,
			msg:         "both HTTP and HTTPS servers",
		},
	}

	for _, tc := range testcases {
		cfg := string(executeServers(tc.conf))

		defaultSSLExists := strings.Contains(cfg, "listen 443 ssl default_server")
		defaultHTTPExists := strings.Contains(cfg, "listen 80 default_server")

		if tc.sslDefault && !defaultSSLExists {
			t.Errorf(
				"executeServers() did not generate a config with a default TLS termination server for test: %q",
				tc.msg,
			)
		}

		if !tc.sslDefault && defaultSSLExists {
			t.Errorf("executeServers() generated a config with a default TLS termination server for test: %q", tc.msg)
		}

		if tc.httpDefault && !defaultHTTPExists {
			t.Errorf("executeServers() did not generate a config with a default http server for test: %q", tc.msg)
		}

		if !tc.httpDefault && defaultHTTPExists {
			t.Errorf("executeServers() generated a config with a default http server for test: %q", tc.msg)
		}

		if len(cfg) == 0 {
			t.Errorf("executeServers() generated empty config for test: %q", tc.msg)
		}
	}
}

func TestCreateServers(t *testing.T) {
	const (
		certPath = "/etc/nginx/secrets/cert"
	)

	hr := &v1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "route1",
		},
		Spec: v1beta1.HTTPRouteSpec{
			Hostnames: []v1beta1.Hostname{
				"cafe.example.com",
			},
			Rules: []v1beta1.HTTPRouteRule{
				{
					// matches with path and methods
					Matches: []v1beta1.HTTPRouteMatch{
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/"),
							},
							Method: helpers.GetHTTPMethodPointer(v1beta1.HTTPMethodPost),
						},
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/"),
							},
							Method: helpers.GetHTTPMethodPointer(v1beta1.HTTPMethodPatch),
						},
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/"), // should generate an "any" httpmatch since other matches exists for /
							},
						},
					},
				},
				{
					// A match with all possible fields set
					Matches: []v1beta1.HTTPRouteMatch{
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/test"),
							},
							Method: helpers.GetHTTPMethodPointer(v1beta1.HTTPMethodGet),
							Headers: []v1beta1.HTTPHeaderMatch{
								{
									Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchExact),
									Name:  "Version",
									Value: "V1",
								},
								{
									Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchExact),
									Name:  "test",
									Value: "foo",
								},
								{
									Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchExact),
									Name:  "my-header",
									Value: "my-value",
								},
							},
							QueryParams: []v1beta1.HTTPQueryParamMatch{
								{
									Type:  helpers.GetQueryParamMatchTypePointer(v1beta1.QueryParamMatchExact),
									Name:  "GrEat", // query names and values should not be normalized to lowercase
									Value: "EXAMPLE",
								},
								{
									Type:  helpers.GetQueryParamMatchTypePointer(v1beta1.QueryParamMatchExact),
									Name:  "test",
									Value: "foo=bar",
								},
							},
						},
					},
				},
				{
					// A match with just path
					Matches: []v1beta1.HTTPRouteMatch{
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/path-only"),
							},
						},
					},
				},
				{
					// A match with a redirect with implicit port
					Matches: []v1beta1.HTTPRouteMatch{
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/redirect-implicit-port"),
							},
						},
					},
					// redirect is set in the corresponding state.MatchRule
				},
				{
					// A match with a redirect with explicit port
					Matches: []v1beta1.HTTPRouteMatch{
						{
							Path: &v1beta1.HTTPPathMatch{
								Value: helpers.GetStringPointer("/redirect-explicit-port"),
							},
						},
					},
					// redirect is set in the corresponding state.MatchRule
				},
			},
		},
	}

	hrNsName := types.NamespacedName{Namespace: hr.Namespace, Name: hr.Name}

	fooGroup := state.BackendGroup{
		Source:  hrNsName,
		RuleIdx: 0,
		Backends: []state.BackendRef{
			{
				Name:   "test_foo_80",
				Valid:  true,
				Weight: 1,
			},
		},
	}

	// barGroup has two backends, which should generate a proxy pass with a variable.
	barGroup := state.BackendGroup{
		Source:  hrNsName,
		RuleIdx: 1,
		Backends: []state.BackendRef{
			{
				Name:   "test_bar_80",
				Valid:  true,
				Weight: 50,
			},
			{
				Name:   "test_bar2_80",
				Valid:  true,
				Weight: 50,
			},
		},
	}

	// baz group has an invalid backend, which should generate a proxy pass to the invalid ref backend.
	bazGroup := state.BackendGroup{
		Source:  hrNsName,
		RuleIdx: 2,
		Backends: []state.BackendRef{
			{
				Name:   "test_baz_80",
				Valid:  false,
				Weight: 1,
			},
		},
	}

	filterGroup1 := state.BackendGroup{Source: hrNsName, RuleIdx: 3}

	filterGroup2 := state.BackendGroup{Source: hrNsName, RuleIdx: 4}

	cafePathRules := []state.PathRule{
		{
			Path: "/",
			MatchRules: []state.MatchRule{
				{
					MatchIdx:     0,
					RuleIdx:      0,
					BackendGroup: fooGroup,
					Source:       hr,
				},
				{
					MatchIdx:     1,
					RuleIdx:      0,
					BackendGroup: fooGroup,
					Source:       hr,
				},
				{
					MatchIdx:     2,
					RuleIdx:      0,
					BackendGroup: fooGroup,
					Source:       hr,
				},
			},
		},
		{
			Path: "/test",
			MatchRules: []state.MatchRule{
				{
					MatchIdx:     0,
					RuleIdx:      1,
					BackendGroup: barGroup,
					Source:       hr,
				},
			},
		},
		{
			Path: "/path-only",
			MatchRules: []state.MatchRule{
				{
					MatchIdx:     0,
					RuleIdx:      2,
					BackendGroup: bazGroup,
					Source:       hr,
				},
			},
		},
		{
			Path: "/redirect-implicit-port",
			MatchRules: []state.MatchRule{
				{
					MatchIdx: 0,
					RuleIdx:  3,
					Source:   hr,
					Filters: state.Filters{
						RequestRedirect: &v1beta1.HTTPRequestRedirectFilter{
							Hostname: (*v1beta1.PreciseHostname)(helpers.GetStringPointer("foo.example.com")),
						},
					},
					BackendGroup: filterGroup1,
				},
			},
		},
		{
			Path: "/redirect-explicit-port",
			MatchRules: []state.MatchRule{
				{
					MatchIdx: 0,
					RuleIdx:  4,
					Source:   hr,
					Filters: state.Filters{
						RequestRedirect: &v1beta1.HTTPRequestRedirectFilter{
							Hostname: (*v1beta1.PreciseHostname)(helpers.GetStringPointer("bar.example.com")),
							Port:     (*v1beta1.PortNumber)(helpers.GetInt32Pointer(8080)),
						},
					},
					BackendGroup: filterGroup2,
				},
			},
		},
	}

	httpServers := []state.VirtualServer{
		{
			Hostname:  "cafe.example.com",
			PathRules: cafePathRules,
		},
	}

	sslServers := []state.VirtualServer{
		{
			Hostname:  "cafe.example.com",
			SSL:       &state.SSL{CertificatePath: certPath},
			PathRules: cafePathRules,
		},
	}

	expectedMatchString := func(m []httpMatch) string {
		b, err := json.Marshal(m)
		if err != nil {
			t.Errorf("error marshaling test match: %v", err)
		}
		return string(b)
	}

	slashMatches := []httpMatch{
		{Method: v1beta1.HTTPMethodPost, RedirectPath: "/_route0"},
		{Method: v1beta1.HTTPMethodPatch, RedirectPath: "/_route1"},
		{Any: true, RedirectPath: "/_route2"},
	}
	testMatches := []httpMatch{
		{
			Method:       v1beta1.HTTPMethodGet,
			Headers:      []string{"Version:V1", "test:foo", "my-header:my-value"},
			QueryParams:  []string{"GrEat=EXAMPLE", "test=foo=bar"},
			RedirectPath: "/test_route0",
		},
	}

	getExpectedLocations := func(isHTTPS bool) []http.Location {
		port := 80
		if isHTTPS {
			port = 443
		}

		return []http.Location{
			{
				Path:      "/_route0",
				Internal:  true,
				ProxyPass: "http://test_foo_80",
			},
			{
				Path:      "/_route1",
				Internal:  true,
				ProxyPass: "http://test_foo_80",
			},
			{
				Path:      "/_route2",
				Internal:  true,
				ProxyPass: "http://test_foo_80",
			},
			{
				Path:         "/",
				HTTPMatchVar: expectedMatchString(slashMatches),
			},
			{
				Path:      "/test_route0",
				Internal:  true,
				ProxyPass: "http://$test_route1_rule1",
			},
			{
				Path:         "/test",
				HTTPMatchVar: expectedMatchString(testMatches),
			},
			{
				Path:      "/path-only",
				ProxyPass: "http://invalid-backend-ref",
			},
			{
				Path: "/redirect-implicit-port",
				Return: &http.Return{
					Code: 302,
					URL:  fmt.Sprintf("$scheme://foo.example.com:%d$request_uri", port),
				},
			},
			{
				Path: "/redirect-explicit-port",
				Return: &http.Return{
					Code: 302,
					URL:  "$scheme://bar.example.com:8080$request_uri",
				},
			},
		}
	}

	expectedServers := []http.Server{
		{
			IsDefaultHTTP: true,
		},
		{
			IsDefaultSSL: true,
		},
		{
			ServerName: "cafe.example.com",
			Locations:  getExpectedLocations(false),
		},
		{
			ServerName: "cafe.example.com",
			SSL:        &http.SSL{Certificate: certPath, CertificateKey: certPath},
			Locations:  getExpectedLocations(true),
		},
	}

	conf := state.Configuration{
		HTTPServers: httpServers,
		SSLServers:  sslServers,
	}

	result := createServers(conf)

	if diff := cmp.Diff(expectedServers, result); diff != "" {
		t.Errorf("createServers() mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateReturnValForRedirectFilter(t *testing.T) {
	const listenerPort = 123

	tests := []struct {
		filter   *v1beta1.HTTPRequestRedirectFilter
		expected *http.Return
		msg      string
	}{
		{
			filter:   nil,
			expected: nil,
			msg:      "filter is nil",
		},
		{
			filter: &v1beta1.HTTPRequestRedirectFilter{},
			expected: &http.Return{
				Code: http.StatusFound,
				URL:  "$scheme://$host:123$request_uri",
			},
			msg: "all fields are empty",
		},
		{
			filter: &v1beta1.HTTPRequestRedirectFilter{
				Scheme:     helpers.GetStringPointer("https"),
				Hostname:   (*v1beta1.PreciseHostname)(helpers.GetStringPointer("foo.example.com")),
				Port:       (*v1beta1.PortNumber)(helpers.GetInt32Pointer(2022)),
				StatusCode: helpers.GetIntPointer(101),
			},
			expected: &http.Return{
				Code: 101,
				URL:  "https://foo.example.com:2022$request_uri",
			},
			msg: "all fields are set",
		},
	}

	for _, test := range tests {
		result := createReturnValForRedirectFilter(test.filter, listenerPort)
		if diff := cmp.Diff(test.expected, result); diff != "" {
			t.Errorf("createReturnValForRedirectFilter() mismatch %q (-want +got):\n%s", test.msg, diff)
		}
	}
}

func TestCreateHTTPMatch(t *testing.T) {
	testPath := "/internal_loc"

	testPathMatch := v1beta1.HTTPPathMatch{Value: helpers.GetStringPointer("/")}
	testMethodMatch := helpers.GetHTTPMethodPointer(v1beta1.HTTPMethodPut)
	testHeaderMatches := []v1beta1.HTTPHeaderMatch{
		{
			Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchExact),
			Name:  "header-1",
			Value: "val-1",
		},
		{
			Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchExact),
			Name:  "header-2",
			Value: "val-2",
		},
		{
			// regex type is not supported. This should not be added to the httpMatch headers.
			Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchRegularExpression),
			Name:  "ignore-this-header",
			Value: "val",
		},
		{
			Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchExact),
			Name:  "header-3",
			Value: "val-3",
		},
	}

	testDuplicateHeaders := make([]v1beta1.HTTPHeaderMatch, 0, 5)
	duplicateHeaderMatch := v1beta1.HTTPHeaderMatch{
		Type:  helpers.GetHeaderMatchTypePointer(v1beta1.HeaderMatchExact),
		Name:  "HEADER-2", // header names are case-insensitive
		Value: "val-2",
	}
	testDuplicateHeaders = append(testDuplicateHeaders, testHeaderMatches...)
	testDuplicateHeaders = append(testDuplicateHeaders, duplicateHeaderMatch)

	testQueryParamMatches := []v1beta1.HTTPQueryParamMatch{
		{
			Type:  helpers.GetQueryParamMatchTypePointer(v1beta1.QueryParamMatchExact),
			Name:  "arg1",
			Value: "val1",
		},
		{
			Type:  helpers.GetQueryParamMatchTypePointer(v1beta1.QueryParamMatchExact),
			Name:  "arg2",
			Value: "val2=another-val",
		},
		{
			// regex type is not supported. This should not be added to the httpMatch args
			Type:  helpers.GetQueryParamMatchTypePointer(v1beta1.QueryParamMatchRegularExpression),
			Name:  "ignore-this-arg",
			Value: "val",
		},
		{
			Type:  helpers.GetQueryParamMatchTypePointer(v1beta1.QueryParamMatchExact),
			Name:  "arg3",
			Value: "==val3",
		},
	}

	expectedHeaders := []string{"header-1:val-1", "header-2:val-2", "header-3:val-3"}
	expectedArgs := []string{"arg1=val1", "arg2=val2=another-val", "arg3===val3"}

	tests := []struct {
		match    v1beta1.HTTPRouteMatch
		expected httpMatch
		msg      string
	}{
		{
			match: v1beta1.HTTPRouteMatch{
				Path: &testPathMatch,
			},
			expected: httpMatch{
				Any:          true,
				RedirectPath: testPath,
			},
			msg: "path only match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Path:   &testPathMatch, // A path match with a method should not set the Any field to true
				Method: testMethodMatch,
			},
			expected: httpMatch{
				Method:       "PUT",
				RedirectPath: testPath,
			},
			msg: "method only match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Headers: testHeaderMatches,
			},
			expected: httpMatch{
				RedirectPath: testPath,
				Headers:      expectedHeaders,
			},
			msg: "headers only match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				QueryParams: testQueryParamMatches,
			},
			expected: httpMatch{
				QueryParams:  expectedArgs,
				RedirectPath: testPath,
			},
			msg: "query params only match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Method:      testMethodMatch,
				QueryParams: testQueryParamMatches,
			},
			expected: httpMatch{
				Method:       "PUT",
				QueryParams:  expectedArgs,
				RedirectPath: testPath,
			},
			msg: "method and query params match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Method:  testMethodMatch,
				Headers: testHeaderMatches,
			},
			expected: httpMatch{
				Method:       "PUT",
				Headers:      expectedHeaders,
				RedirectPath: testPath,
			},
			msg: "method and headers match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				QueryParams: testQueryParamMatches,
				Headers:     testHeaderMatches,
			},
			expected: httpMatch{
				QueryParams:  expectedArgs,
				Headers:      expectedHeaders,
				RedirectPath: testPath,
			},
			msg: "query params and headers match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Headers:     testHeaderMatches,
				QueryParams: testQueryParamMatches,
				Method:      testMethodMatch,
			},
			expected: httpMatch{
				Method:       "PUT",
				Headers:      expectedHeaders,
				QueryParams:  expectedArgs,
				RedirectPath: testPath,
			},
			msg: "method, headers, and query params match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Headers: testDuplicateHeaders,
			},
			expected: httpMatch{
				Headers:      expectedHeaders,
				RedirectPath: testPath,
			},
			msg: "duplicate header names",
		},
	}
	for _, tc := range tests {
		result := createHTTPMatch(tc.match, testPath)
		if diff := helpers.Diff(result, tc.expected); diff != "" {
			t.Errorf("createHTTPMatch() returned incorrect httpMatch for test case: %q, diff: %+v", tc.msg, diff)
		}
	}
}

func TestCreateQueryParamKeyValString(t *testing.T) {
	expected := "key=value"

	result := createQueryParamKeyValString(
		v1beta1.HTTPQueryParamMatch{
			Name:  "key",
			Value: "value",
		},
	)
	if result != expected {
		t.Errorf("createQueryParamKeyValString() returned %q but expected %q", result, expected)
	}

	expected = "KeY=vaLUe=="

	result = createQueryParamKeyValString(
		v1beta1.HTTPQueryParamMatch{
			Name:  "KeY",
			Value: "vaLUe==",
		},
	)
	if result != expected {
		t.Errorf("createQueryParamKeyValString() returned %q but expected %q", result, expected)
	}
}

func TestCreateHeaderKeyValString(t *testing.T) {
	expected := "kEy:vALUe"

	result := createHeaderKeyValString(
		v1beta1.HTTPHeaderMatch{
			Name:  "kEy",
			Value: "vALUe",
		},
	)

	if result != expected {
		t.Errorf("createHeaderKeyValString() returned %q but expected %q", result, expected)
	}
}

func TestIsPathOnlyMatch(t *testing.T) {
	tests := []struct {
		match    v1beta1.HTTPRouteMatch
		expected bool
		msg      string
	}{
		{
			match: v1beta1.HTTPRouteMatch{
				Path: &v1beta1.HTTPPathMatch{
					Value: helpers.GetStringPointer("/path"),
				},
			},
			expected: true,
			msg:      "path only match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Path: &v1beta1.HTTPPathMatch{
					Value: helpers.GetStringPointer("/path"),
				},
				Method: helpers.GetHTTPMethodPointer(v1beta1.HTTPMethodGet),
			},
			expected: false,
			msg:      "method defined in match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Path: &v1beta1.HTTPPathMatch{
					Value: helpers.GetStringPointer("/path"),
				},
				Headers: []v1beta1.HTTPHeaderMatch{
					{
						Name:  "header",
						Value: "val",
					},
				},
			},
			expected: false,
			msg:      "headers defined in match",
		},
		{
			match: v1beta1.HTTPRouteMatch{
				Path: &v1beta1.HTTPPathMatch{
					Value: helpers.GetStringPointer("/path"),
				},
				QueryParams: []v1beta1.HTTPQueryParamMatch{
					{
						Name:  "arg",
						Value: "val",
					},
				},
			},
			expected: false,
			msg:      "query params defined in match",
		},
	}

	for _, tc := range tests {
		result := isPathOnlyMatch(tc.match)

		if result != tc.expected {
			t.Errorf("isPathOnlyMatch() returned %t but expected %t for test case %q", result, tc.expected, tc.msg)
		}
	}
}

func TestCreateProxyPass(t *testing.T) {
	expected := "http://10.0.0.1:80"

	result := createProxyPass("10.0.0.1:80")
	if result != expected {
		t.Errorf("createProxyPass() returned %s but expected %s", result, expected)
	}
}

func TestCreateProxyPassForVar(t *testing.T) {
	expected := "http://$my_variable"

	result := createProxyPassForVar("my-variable")
	if result != expected {
		t.Errorf("createProxyPassForVar() returned %s but expected %s", result, expected)
	}
}

func TestCreateMatchLocation(t *testing.T) {
	expected := http.Location{
		Path:     "/path",
		Internal: true,
	}

	result := createMatchLocation("/path")
	if result != expected {
		t.Errorf("createMatchLocation() returned %v but expected %v", result, expected)
	}
}

func TestCreatePathForMatch(t *testing.T) {
	expected := "/path_route1"

	result := createPathForMatch("/path", 1)
	if result != expected {
		t.Errorf("createPathForMatch() returned %q but expected %q", result, expected)
	}
}
