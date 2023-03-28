package proxy

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
	"k8s.io/client-go/rest"
)

func New(cfg *rest.Config, b bundle.Bundle) http.Handler {
	proxyHandler, err := ReverseProxyForApiServerHandler(cfg)
	if err != nil {
		log.Fatalln(err)
	}
	proxyHandler.ModifyResponse = RewriteResponseResourceFields

	r := mux.NewRouter()
	r.Handle("/api/v1/namespaces/{namespace}/pods/{pod}/log", LogsHandler(b))
	r.PathPrefix("/").Handler(proxyHandler)
	return r
}
