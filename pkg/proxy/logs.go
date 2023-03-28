package proxy

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

// LogsHandler serves logs for k8s `logs` subresource from the provided bundle.
func LogsHandler(b bundle.Bundle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		filename := fmt.Sprintf("%s-%s.log", vars["pod"], r.URL.Query().Get("container"))
		podLogsPath := filepath.Join(b.Layout().PodLogs(), vars["namespace"], filename)
		data, err := afero.ReadFile(b, podLogsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO(mh): inject logger
		log.Printf("LogsHandler: %s served from %q\n", r.URL, podLogsPath)
		if _, err := w.Write(data); err != nil {
			log.Printf("LogsHandler: failed to write response data: %s", err)
		}
	}
}
