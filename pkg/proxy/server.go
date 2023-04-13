package proxy

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
)

// New create new proxy handler that can be used by HTTP library.
func New(cfg *rest.Config, b bundle.Bundle, rr rewriter.ResourceRewriter) http.Handler {
	proxyHandler, err := ReverseProxyForAPIServerHandler(cfg)
	if err != nil {
		log.Fatalln(err)
	}
	// disable bodyclose linting as it seems like false positive
	// https://github.com/timakin/bodyclose/issues/42
	proxyHandler.ModifyResponse = proxyModifyResponse(rr) //nolint:bodyclose // false positive

	r := mux.NewRouter()
	r.Handle("/api/v1/namespaces/{namespace}/pods/{pod}/log", LogsHandler(b))
	r.PathPrefix("/").Handler(proxyHandler)
	return r
}
