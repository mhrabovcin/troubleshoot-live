package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

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
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		preferJSONOverProtobuf(req)
	}
	proxy.Transport = transport
	return proxy, nil
}

func preferJSONOverProtobuf(req *http.Request) {
	accept := req.Header.Get("Accept")
	if accept == "" || !strings.Contains(accept, "application/vnd.kubernetes.protobuf") {
		return
	}

	acceptedTypes := strings.Split(accept, ",")
	jsonAcceptedTypes := make([]string, 0, len(acceptedTypes))
	for _, acceptedType := range acceptedTypes {
		acceptedType = strings.TrimSpace(acceptedType)
		if acceptedType == "" || strings.Contains(acceptedType, "application/vnd.kubernetes.protobuf") {
			continue
		}
		jsonAcceptedTypes = append(jsonAcceptedTypes, acceptedType)
	}

	if len(jsonAcceptedTypes) == 0 {
		req.Header.Set("Accept", "application/json")
		return
	}
	req.Header.Set("Accept", strings.Join(jsonAcceptedTypes, ", "))
}
