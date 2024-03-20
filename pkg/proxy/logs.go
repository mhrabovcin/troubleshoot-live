package proxy

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

// LogsHandler serves logs for k8s `logs` subresource from the provided bundle.
func LogsHandler(b bundle.Bundle, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		podLogsPath := ""

		// Search for pod logs path in the bundle which could be collected either by the
		// pod logs collector or by the cluster resources collector, which collects pod logs
		// for failing pods.
		filename := fmt.Sprintf("%s-%s.log", vars["pod"], r.URL.Query().Get("container"))
		candidatePaths := []string{
			filepath.Join(b.Layout().PodLogs(), vars["namespace"], filename),
			filepath.Join(b.Layout().ClusterResources(), "pods/logs", vars["namespace"], vars["pod"], r.URL.Query().Get("container")+".log"),
		}
		for _, candidatePath := range candidatePaths {
			if exists, _ := afero.Exists(b, candidatePath); exists {
				podLogsPath = candidatePath
				break
			}
		}

		if podLogsPath == "" {
			http.Error(w, "pod logs not found in the bundle", http.StatusInternalServerError)
			return
		}

		data, err := afero.ReadFile(b, podLogsPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		l := l.With("url", r.URL, "logs source", podLogsPath)

		// By default the `k9s` requests logs prefixed with timestamp and in the logs pane
		// only displays a portion without the timestamp, by cutting prefix separated by first
		// space byte(' '). The troubleshoot.sh requests logs without timestamps, which causes
		// issues in the logs pane and for some pods the logs are cut from beginnging.
		// This will backfill zeroed timestamp for each line.
		if r.URL.Query().Get("timestamps") == "true" {
			lines := bytes.Split(data, []byte("\n"))
			timestampPrefixRegexp := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{6})?Z `)
			if !timestampPrefixRegexp.Match(lines[0]) {
				l.Debug("adding timestamp prefix to logs")
				zeroTime := []byte(time.UnixMicro(0).Format(time.RFC3339Nano))
				// Add prefix to each line.
				for i := range lines {
					lines[i] = bytes.Join([][]byte{zeroTime, lines[i]}, []byte{' '})
				}
				data = bytes.Join(lines, []byte("\n"))
			}
		}

		l.Debug("serving logs")
		if _, err := w.Write(data); err != nil {
			slog.Error("failed to write response data", "err", err)
		}
	}
}
