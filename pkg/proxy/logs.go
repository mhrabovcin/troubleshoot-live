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

// TODO() inject logger
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

		log.Printf("LogsHandler: %s served from %q\n", r.URL, podLogsPath)
		w.Write(data)
	}
}
