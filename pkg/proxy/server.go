package proxy

import (
	"fmt"
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
	r.Use(loggingMiddleware)
	r.Handle("/api/v1/namespaces/{namespace}/pods/{pod}/log", LogsHandler(b))
	r.PathPrefix("/").Handler(proxyHandler)
	return r
}

type LogRecorder struct {
	http.ResponseWriter
	status int
}

func (r *LogRecorder) Write(p []byte) (int, error) {
	return r.ResponseWriter.Write(p)
}

func (r *LogRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writerWrap := &LogRecorder{
			ResponseWriter: w,
		}
		next.ServeHTTP(writerWrap, r)
		fmt.Println(writerWrap.status, r.Method, r.RequestURI)
	})
}
