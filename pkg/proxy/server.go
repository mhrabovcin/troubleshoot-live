package proxy

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

// New create new proxy handler that can be used by HTTP library.
func New(cfg *rest.Config, b bundle.Bundle) http.Handler {
	proxyHandler, err := ReverseProxyForAPIServerHandler(cfg)
	if err != nil {
		log.Fatalln(err)
	}
	proxyHandler.ModifyResponse = rewriteResponseResourceFields

	r := mux.NewRouter()
	r.Handle("/api/v1/namespaces/{namespace}/pods/{pod}/log", LogsHandler(b))
	r.PathPrefix("/").Handler(proxyHandler)
	return r
}
