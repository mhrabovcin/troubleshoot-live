package proxy

import (
	"net/http"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

func TestNormalizeHTTPPrefix(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		expected  string
		expectErr bool
	}{
		{name: "empty", in: "", expected: ""},
		{name: "root", in: "/", expected: ""},
		{name: "without-leading-slash", in: "proxy", expected: "/proxy"},
		{name: "with-trailing-slash", in: "/proxy/", expected: "/proxy"},
		{name: "nested", in: "/proxy/v1", expected: "/proxy/v1"},
		{name: "invalid-query", in: "/proxy?a=b", expectErr: true},
		{name: "invalid-fragment", in: "/proxy#x", expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeHTTPPrefix(tt.in)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestProxyWithPrefixStripsPrefixBeforeForwarding(t *testing.T) {
	gotPath := ""
	proxyTarget := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	h := newRouterWithPrefix(
		"/proxy",
		bundle.FromFs(afero.NewMemMapFs()),
		proxyTarget,
	)
	req := httptest.NewRequest(http.MethodGet, "/proxy/api/v1/pods", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/api/v1/pods", gotPath)
}
