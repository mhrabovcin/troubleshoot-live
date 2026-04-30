package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
)

func proxyModifyResponse(rr rewriter.ResourceRewriter) func(*http.Response) error {
	r := &resourceRewriter{rewriter: rr}
	return r.rewriteResponseResourceFields
}

type resourceRewriter struct {
	rewriter rewriter.ResourceRewriter
}

func (rr *resourceRewriter) rewriteResponseResourceFields(r *http.Response) (returnErr error) {
	if r.StatusCode != http.StatusOK {
		return nil
	}

	if !isJSONContentType(r.Header.Get("content-type")) {
		return nil
	}

	if isWatchResponse(r) {
		return rr.rewriteWatchResponse(r)
	}

	data, err := readResponseBody(r)
	if err != nil {
		return err
	}

	defer func() {
		returnErr = writeResponseBody(r, data)
	}()

	list := &unstructured.UnstructuredList{}
	// The condition for items > 0 is required in order to avoid processing non
	// list requests.
	if err := json.Unmarshal(data, &list); err == nil && len(list.Items) > 0 {
		err := list.EachListItem(func(o runtime.Object) error {
			if err := remapFields(o, rr.rewriter); err != nil {
				log.Println(err)
			}
			return nil
		})
		if err == nil {
			data, _ = json.Marshal(list)
		}
	}

	// TODO: how to handle the table out
	// table := &metav1.Table{}
	// if err := json.Unmarshal(data, &table); err == nil {
	// 	fmt.Printf("%#v\n", table.ColumnDefinitions)
	// 	if len(table.Rows) > 0 {
	// 		row := table.Rows[0]
	// 		fmt.Printf("object example: %#v\n", row.Object)
	// 	}
	// }

	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, &u); err == nil {
		if err := remapFields(u, rr.rewriter); err != nil {
			log.Println(err)
			return nil
		}
		data, _ = json.Marshal(u)
	}

	return nil
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return contentType == "application/json"
	}
	return mediaType == "application/json"
}

func isWatchResponse(r *http.Response) bool {
	if r.Request == nil || r.Request.URL == nil {
		return false
	}

	return isTruthy(r.Request.URL.Query().Get("watch"))
}

func isTruthy(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true":
		return true
	default:
		return false
	}
}

func (rr *resourceRewriter) rewriteWatchResponse(r *http.Response) error {
	source := r.Body
	reader := io.Reader(source)

	var gzipReader *gzip.Reader
	if isGzipped(r) {
		var err error
		gzipReader, err = gzip.NewReader(source)
		if err != nil {
			return err
		}
		reader = gzipReader
	}

	pipeReader, pipeWriter := io.Pipe()
	r.Body = pipeReader
	r.ContentLength = -1
	r.Header.Del("Content-Length")

	go rr.streamWatchEvents(reader, source, gzipReader, pipeWriter, isGzipped(r))

	return nil
}

func (rr *resourceRewriter) streamWatchEvents(reader io.Reader, source io.Closer, gzipReader *gzip.Reader, pipeWriter *io.PipeWriter, gzipOutput bool) {
	defer source.Close()
	if gzipReader != nil {
		defer gzipReader.Close()
	}

	writer := io.Writer(pipeWriter)
	var gzipWriter *gzip.Writer
	if gzipOutput {
		gzipWriter = gzip.NewWriter(pipeWriter)
		writer = gzipWriter
	}

	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(writer)

	for {
		event := map[string]json.RawMessage{}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				closePipeWriter(pipeWriter, gzipWriter, nil)
				return
			}
			closePipeWriter(pipeWriter, gzipWriter, err)
			return
		}

		if err := rr.rewriteWatchEvent(event); err != nil {
			closePipeWriter(pipeWriter, gzipWriter, err)
			return
		}

		if err := encoder.Encode(event); err != nil {
			closePipeWriter(pipeWriter, gzipWriter, err)
			return
		}
		if gzipWriter != nil {
			if err := gzipWriter.Flush(); err != nil {
				closePipeWriter(pipeWriter, gzipWriter, err)
				return
			}
		}
	}
}

func closePipeWriter(pipeWriter *io.PipeWriter, gzipWriter *gzip.Writer, err error) {
	if gzipWriter != nil {
		if closeErr := gzipWriter.Close(); err == nil {
			err = closeErr
		}
	}

	if err != nil {
		_ = pipeWriter.CloseWithError(err)
		return
	}

	_ = pipeWriter.Close()
}

func (rr *resourceRewriter) rewriteWatchEvent(event map[string]json.RawMessage) error {
	objectData := bytes.TrimSpace(event["object"])
	if len(objectData) == 0 || bytes.Equal(objectData, []byte("null")) || objectData[0] != '{' {
		return nil
	}

	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(objectData, u); err != nil {
		return err
	}
	if err := remapFields(u, rr.rewriter); err != nil {
		log.Println(err)
		return nil
	}

	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	event["object"] = data
	return nil
}

func isGzipped(r *http.Response) bool {
	return r.Header.Get("content-encoding") == "gzip"
}

func readResponseBody(r *http.Response) ([]byte, error) {
	reader := r.Body
	if isGzipped(r) {
		var err error
		reader, err = gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
	}
	defer r.Body.Close()

	return io.ReadAll(reader)
}

func writeResponseBody(r *http.Response, data []byte) error {
	if isGzipped(r) {
		gzipped := &bytes.Buffer{}
		gzipWriter := gzip.NewWriter(gzipped)
		_, err := io.Copy(gzipWriter, bytes.NewReader(data))
		if err != nil {
			return err
		}
		gzipWriter.Close()
		data = gzipped.Bytes()
	}

	r.Body = io.NopCloser(bytes.NewReader(data))
	r.ContentLength = int64(len(data))
	r.Header.Set("Content-Length", strconv.Itoa(len(data)))
	return nil
}

func remapFields(in runtime.Object, rr rewriter.ResourceRewriter) error {
	if rr == nil {
		return fmt.Errorf("resource rewriter missing")
	}

	u, ok := in.(*unstructured.Unstructured)
	if !ok {
		// TODO(mh): handle non-unstructured objects
		return nil
	}

	return rr.BeforeServing(u)
}
