package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

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

	if r.Header.Get("content-type") != "application/json" {
		return nil
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
