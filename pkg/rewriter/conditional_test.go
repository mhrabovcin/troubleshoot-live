package rewriter_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter/mocks"
)

var (
	anyUnstructured = mock.MatchedBy(func(_ *unstructured.Unstructured) bool { return true })
	matchAny        = func(u *unstructured.Unstructured) bool { return true }
	matchNone       = func(u *unstructured.Unstructured) bool { return false }
)

func TestWhen_Matching(t *testing.T) {
	rrMock := mocks.NewResourceRewriter(t)
	rrMock.EXPECT().BeforeImport(anyUnstructured).Return(nil).Once()
	rrMock.EXPECT().BeforeServing(anyUnstructured).Return(nil).Once()
	assert.NoError(t, rewriter.When(matchAny, rrMock).BeforeImport(&unstructured.Unstructured{}))
	assert.NoError(t, rewriter.When(matchAny, rrMock).BeforeServing(&unstructured.Unstructured{}))
}

func TestWhen_Matching_Error(t *testing.T) {
	rrMock := mocks.NewResourceRewriter(t)
	rrMock.EXPECT().BeforeImport(anyUnstructured).Return(fmt.Errorf("test")).Once()
	rrMock.EXPECT().BeforeServing(anyUnstructured).Return(fmt.Errorf("serving")).Once()
	assert.ErrorContains(t, rewriter.When(matchAny, rrMock).BeforeImport(&unstructured.Unstructured{}), "test")
	assert.ErrorContains(t, rewriter.When(matchAny, rrMock).BeforeServing(&unstructured.Unstructured{}), "serving")
}

func TestWhen_NoMatch(t *testing.T) {
	rrMock := mocks.NewResourceRewriter(t)
	assert.NoError(t, rewriter.When(matchNone, rrMock).BeforeImport(&unstructured.Unstructured{}))
	assert.NoError(t, rewriter.When(matchNone, rrMock).BeforeServing(&unstructured.Unstructured{}))
}

func TestCondition_MatchGVK(t *testing.T) {
	testCases := []struct {
		config     schema.GroupVersionKind
		apiVersion string
		kind       string
		expected   bool
	}{
		{
			config:     schema.FromAPIVersionAndKind("v1", "Pod"),
			apiVersion: "v1",
			kind:       "Pod",
			expected:   true,
		},
		{
			config:     schema.FromAPIVersionAndKind("scheduling.k8s.io/v1beta1", "PriorityClass"),
			apiVersion: "scheduling.k8s.io/v1",
			kind:       "PriorityClass",
			expected:   false,
		},
		{
			config:     schema.FromAPIVersionAndKind("scheduling.k8s.io/v1", "PriorityClass"),
			apiVersion: "scheduling.k8s.io/v1",
			kind:       "PriorityClassDifferent",
			expected:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s:%s/%s", tc.config, tc.apiVersion, tc.kind), func(tt *testing.T) {
			condition := rewriter.MatchGVK(tc.config)
			u := &unstructured.Unstructured{}
			u.SetAPIVersion(tc.apiVersion)
			u.SetKind(tc.kind)
			assert.Equal(tt, tc.expected, condition(u))
		})
	}
}
