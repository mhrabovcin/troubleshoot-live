package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mhrabovcin/troubleshoot-live/pkg/importer"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func RewriteResponseResourceFields(r *http.Response) (returnErr error) {
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
			if err := remapFields(o); err != nil {
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
		if err := remapFields(u); err != nil {
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

	return io.ReadAll(reader)
}

func writeResponseBody(r *http.Response, data []byte) error {
	if isGzipped(r) {
		gzipped := &bytes.Buffer{}
		gzipWriter := gzip.NewWriter(gzipped)
		_, err := io.Copy(gzipWriter, bytes.NewReader(data))
		if err != nil {
			return err
		} else {
			gzipWriter.Close()
			data = gzipped.Bytes()
		}
	}

	r.Body = io.NopCloser(bytes.NewReader(data))
	r.ContentLength = int64(len(data))
	r.Header.Set("Content-Length", strconv.Itoa(len(data)))
	return nil
}

func remapFields(in runtime.Object) error {
	o, err := meta.Accessor(in)
	if err != nil {
		return err
	}
	annotations := o.GetAnnotations()
	if originalTime, ok := annotations[importer.AnnotationForOriginalValue("creationTimestamp")]; ok {
		parsedTime, err := time.Parse(time.RFC3339, originalTime)
		if err != nil {
			return nil
		}
		o.SetCreationTimestamp(metav1.NewTime(parsedTime))
		delete(annotations, importer.AnnotationForOriginalValue("creationTimestamp"))
		log.Printf("[%s] %s/%s: resource creationTimestamp modified\n", in.GetObjectKind().GroupVersionKind(), o.GetNamespace(), o.GetName())
	}
	o.SetAnnotations(annotations)

	return nil
}
