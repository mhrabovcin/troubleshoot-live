package rewriter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCRDDisableConversionWebhook_BeforeImportAndRestore(t *testing.T) {
	t.Parallel()

	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name": "widgets.example.com",
			},
			"spec": map[string]any{
				"group": "example.com",
				"conversion": map[string]any{
					"strategy": "Webhook",
					"webhook": map[string]any{
						"conversionReviewVersions": []any{"v1"},
					},
				},
			},
		},
	}

	r := CRDDisableConversionWebhook()
	require.NoError(t, r.BeforeImport(u))

	strategy, found, err := unstructured.NestedString(u.Object, "spec", "conversion", "strategy")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "None", strategy)

	webhook, webhookFound, err := unstructured.NestedFieldNoCopy(u.Object, "spec", "conversion", "webhook")
	require.NoError(t, err)
	assert.True(t, webhookFound)
	assert.Nil(t, webhook)
	assert.Contains(t, u.GetAnnotations(), annotationForField("spec", "conversion"))

	require.NoError(t, r.BeforeServing(u))

	strategy, found, err = unstructured.NestedString(u.Object, "spec", "conversion", "strategy")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "Webhook", strategy)

	webhook, webhookFound, err = unstructured.NestedFieldNoCopy(u.Object, "spec", "conversion", "webhook")
	require.NoError(t, err)
	assert.True(t, webhookFound)
	assert.NotNil(t, webhook)
	assert.NotContains(t, u.GetAnnotations(), annotationForField("spec", "conversion"))
}

func TestCRDDisableConversionWebhook_RestoreMissingConversion(t *testing.T) {
	t.Parallel()

	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name": "widgets.example.com",
			},
			"spec": map[string]any{
				"group": "example.com",
			},
		},
	}

	r := CRDDisableConversionWebhook()
	require.NoError(t, r.BeforeImport(u))

	strategy, found, err := unstructured.NestedString(u.Object, "spec", "conversion", "strategy")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "None", strategy)
	assert.Contains(t, u.GetAnnotations(), annotationForField("spec", "conversion"))

	require.NoError(t, r.BeforeServing(u))

	_, found, err = unstructured.NestedFieldNoCopy(u.Object, "spec", "conversion")
	require.NoError(t, err)
	assert.False(t, found)
	assert.NotContains(t, u.GetAnnotations(), annotationForField("spec", "conversion"))
}

func TestCRDDisableConversionWebhook_SkipNonCRD(t *testing.T) {
	t.Parallel()

	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "cm",
				"namespace": "default",
			},
			"data": map[string]any{
				"k": "v",
			},
		},
	}

	r := CRDDisableConversionWebhook()
	require.NoError(t, r.BeforeImport(u))
	require.NoError(t, r.BeforeServing(u))
	assert.Empty(t, u.GetAnnotations())
}
