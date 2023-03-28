package proxy

import (
	"net/http/httputil"
	"net/url"

	"k8s.io/client-go/rest"
)

// ReverseProxyForAPIServerHandler creates a reverse proxy handler for given
// rest config that represents a running k8s api server.
func ReverseProxyForAPIServerHandler(cfg *rest.Config) (*httputil.ReverseProxy, error) {
	apiServerURL, err := url.Parse(cfg.Host)
	if err != nil {
		return nil, err
	}

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(apiServerURL)
	proxy.Transport = transport
	return proxy, nil
}
