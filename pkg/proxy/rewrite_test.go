package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
)

const originalResourceVersionAnnotation = "troubleshoot-live/metadata.resourceVersion"

func TestRewriteResponseResourceFields_RewritesObjectResponse(t *testing.T) {
	body := `{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {
			"name": "pod-1",
			"resourceVersion": "server-rv",
			"annotations": {
				"troubleshoot-live/metadata.resourceVersion": "\"bundle-rv\""
			}
		}
	}`
	resp := jsonResponse(body, "")

	err := proxyModifyResponse(rewriter.RemoveField("metadata", "resourceVersion"))(resp)
	require.NoError(t, err)

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), resp.ContentLength)
	assert.Equal(t, strconv.Itoa(len(data)), resp.Header.Get("Content-Length"))

	obj := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &obj))
	metadata := obj["metadata"].(map[string]any)
	assert.Equal(t, "bundle-rv", metadata["resourceVersion"])
	assert.NotContains(t, metadata["annotations"], originalResourceVersionAnnotation)
}

func TestRewriteResponseResourceFields_RewritesListResponse(t *testing.T) {
	body := `{
		"apiVersion": "v1",
		"kind": "PodList",
		"items": [{
			"apiVersion": "v1",
			"kind": "Pod",
			"metadata": {
				"name": "pod-1",
				"resourceVersion": "server-rv",
				"annotations": {
					"troubleshoot-live/metadata.resourceVersion": "\"bundle-rv\""
				}
			}
		}]
	}`
	resp := jsonResponse(body, "")

	err := proxyModifyResponse(rewriter.RemoveField("metadata", "resourceVersion"))(resp)
	require.NoError(t, err)

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	list := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &list))
	item := list["items"].([]any)[0].(map[string]any)
	metadata := item["metadata"].(map[string]any)
	assert.Equal(t, "bundle-rv", metadata["resourceVersion"])
	assert.NotContains(t, metadata["annotations"], originalResourceVersionAnnotation)
}

func TestRewriteResponseResourceFields_RewritesWatchListStream(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"ADDED","object":{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod-1","resourceVersion":"server-rv","annotations":{"troubleshoot-live/metadata.resourceVersion":"\"bundle-rv\""}}}}`,
		`{"type":"BOOKMARK"}`,
	}, "\n") + "\n"
	resp := jsonResponse(stream, "/api/v1/pods?watch=true&sendInitialEvents=true")

	err := proxyModifyResponse(rewriter.RemoveField("metadata", "resourceVersion"))(resp)
	require.NoError(t, err)
	require.Equal(t, int64(-1), resp.ContentLength)
	assert.Empty(t, resp.Header.Get("Content-Length"))

	decoder := json.NewDecoder(resp.Body)

	added := map[string]json.RawMessage{}
	require.NoError(t, decoder.Decode(&added))
	assert.JSONEq(t, `"ADDED"`, string(added["type"]))

	object := map[string]any{}
	require.NoError(t, json.Unmarshal(added["object"], &object))
	metadata := object["metadata"].(map[string]any)
	assert.Equal(t, "bundle-rv", metadata["resourceVersion"])
	assert.NotContains(t, metadata["annotations"], originalResourceVersionAnnotation)

	bookmark := map[string]json.RawMessage{}
	require.NoError(t, decoder.Decode(&bookmark))
	assert.JSONEq(t, `"BOOKMARK"`, string(bookmark["type"]))
	assert.NotContains(t, bookmark, "object")
}

func TestRewriteResponseResourceFields_WatchStreamDoesNotWaitForEOF(t *testing.T) {
	upstreamReader, upstreamWriter := io.Pipe()
	resp := jsonResponse("", "/api/v1/pods?watch=true&sendInitialEvents=true")
	resp.Body = upstreamReader

	err := proxyModifyResponse(rewriter.RemoveField("metadata", "resourceVersion"))(resp)
	require.NoError(t, err)
	defer resp.Body.Close()

	decoded := make(chan map[string]json.RawMessage, 1)
	decodeErr := make(chan error, 1)
	go func() {
		event := map[string]json.RawMessage{}
		if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
			decodeErr <- err
			return
		}
		decoded <- event
	}()

	_, err = upstreamWriter.Write([]byte(`{"type":"ADDED","object":{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod-1","resourceVersion":"server-rv","annotations":{"troubleshoot-live/metadata.resourceVersion":"\"bundle-rv\""}}}}` + "\n"))
	require.NoError(t, err)
	defer upstreamWriter.Close()

	select {
	case err := <-decodeErr:
		require.NoError(t, err)
	case event := <-decoded:
		object := map[string]any{}
		require.NoError(t, json.Unmarshal(event["object"], &object))
		metadata := object["metadata"].(map[string]any)
		assert.Equal(t, "bundle-rv", metadata["resourceVersion"])
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for first watch event before upstream EOF")
	}
}

func TestRewriteResponseResourceFields_RewritesGzipWatchStream(t *testing.T) {
	stream := `{"type":"ADDED","object":{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod-1","resourceVersion":"server-rv","annotations":{"troubleshoot-live/metadata.resourceVersion":"\"bundle-rv\""}}}}` + "\n"
	resp := jsonResponse(gzipString(t, stream), "/api/v1/pods?watch=true")
	resp.Header.Set("Content-Encoding", "gzip")

	err := proxyModifyResponse(rewriter.RemoveField("metadata", "resourceVersion"))(resp)
	require.NoError(t, err)
	assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	reader, err := gzip.NewReader(resp.Body)
	require.NoError(t, err)
	defer reader.Close()

	event := map[string]json.RawMessage{}
	require.NoError(t, json.NewDecoder(reader).Decode(&event))
	object := map[string]any{}
	require.NoError(t, json.Unmarshal(event["object"], &object))
	metadata := object["metadata"].(map[string]any)
	assert.Equal(t, "bundle-rv", metadata["resourceVersion"])
}

func jsonResponse(body string, requestPath string) *http.Response {
	if requestPath == "" {
		requestPath = "/api/v1/pods"
	}

	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       httptest.NewRequest(http.MethodGet, requestPath, nil),
	}
}

func gzipString(t *testing.T, data string) string {
	t.Helper()

	buf := &bytes.Buffer{}
	writer := gzip.NewWriter(buf)
	_, err := writer.Write([]byte(data))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buf.String()
}
