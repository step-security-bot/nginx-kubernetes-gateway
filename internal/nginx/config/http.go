package config

// HTTPServers holds the list of HTTP servers.
type HTTPServers struct {
	Servers []Server
}

// HTTPUpstreams holds the list of HTTP upstreams.
type HTTPUpstreams struct {
	Upstreams []Upstream
}

// Server holds all configuration for an HTTP server.
type Server struct {
	SSL           *SSL
	ServerName    string
	Locations     []Location
	IsDefaultHTTP bool
	IsDefaultSSL  bool
}

// Location holds all configuration for an HTTP location.
type Location struct {
	Return       *Return
	Path         string
	ProxyPass    string
	HTTPMatchVar string
	Internal     bool
}

// Return represents an HTTP return.
type Return struct {
	Code StatusCode
	URL  string
}

// SSL holds all SSL related configuration.
type SSL struct {
	Certificate    string
	CertificateKey string
}

// StatusCode is an HTTP status code.
type StatusCode int

const (
	StatusFound StatusCode = 302
	// StatusNotFound is the HTTP 404 status code.
	StatusNotFound StatusCode = 404
)

// Upstream holds all configuration for an HTTP upstream.
type Upstream struct {
	Name    string
	Servers []UpstreamServer
}

// UpstreamServer holds all configuration for an HTTP upstream server.
type UpstreamServer struct {
	Address string
}
