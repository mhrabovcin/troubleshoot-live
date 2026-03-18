package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
)

// NormalizeHTTPPrefix returns normalized proxy HTTP prefix or empty string.
func NormalizeHTTPPrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}

	u, err := url.Parse(prefix)
	if err != nil {
		return "", err
	}
	if u.Scheme != "" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid http prefix %q: must be path only", prefix)
	}

	p := u.Path
	if p == "" || p == "/" {
		return "", nil
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" || p == "/" {
		return "", nil
	}
	return p, nil
}

// New create new proxy handler that can be used by HTTP library.
func New(cfg *rest.Config, b bundle.Bundle, rr rewriter.ResourceRewriter, httpPrefix string) (http.Handler, error) {
	proxyHandler, err := ReverseProxyForAPIServerHandler(cfg)
	if err != nil {
		return nil, err
	}

	prefix, err := NormalizeHTTPPrefix(httpPrefix)
	if err != nil {
		return nil, err
	}

	// disable bodyclose linting as it seems like false positive
	// https://github.com/timakin/bodyclose/issues/42
	proxyHandler.ModifyResponse = proxyModifyResponse(rr) //nolint:bodyclose // false positive

	return newRouterWithPrefix(prefix, b, proxyHandler), nil
}

func newRouterWithPrefix(prefix string, b bundle.Bundle, proxyHandler http.Handler) http.Handler {
	r := mux.NewRouter()
	if prefix == "" {
		r.Handle("/api/v1/namespaces/{namespace}/pods/{pod}/log", LogsHandler(b, slog.With("handler", "LogsHandler")))
		r.PathPrefix("/").Handler(proxyHandler)
		return r
	}

	subrouter := r.PathPrefix(prefix).Subrouter()
	subrouter.Handle("/api/v1/namespaces/{namespace}/pods/{pod}/log", LogsHandler(b, slog.With("handler", "LogsHandler")))
	subrouter.PathPrefix("/").Handler(http.StripPrefix(prefix, proxyHandler))

	return r
}
