package common

import (
	"context"
	"testing"

	"github.com/kyverno/kyverno/api/kyverno"
	kyvernov2 "github.com/kyverno/kyverno/api/kyverno/v2"
	"github.com/kyverno/kyverno/pkg/client/clientset/versioned/fake"
	"github.com/kyverno/kyverno/pkg/config"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestResourceSpecFromUnstructured(t *testing.T) {
	tests := []struct {
		name     string
		obj      unstructured.Unstructured
		wantKind string
		wantName string
		wantNS   string
		wantAPI  string
	}{
		{
			name: "pod resource",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      "test-pod",
						"namespace": "default",
						"uid":       "abc-123",
					},
				},
			},
			wantKind: "Pod",
			wantName: "test-pod",
			wantNS:   "default",
			wantAPI:  "v1",
		},
		{
			name: "deployment resource",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "my-app",
						"namespace": "production",
						"uid":       "def-456",
					},
				},
			},
			wantKind: "Deployment",
			wantName: "my-app",
			wantNS:   "production",
			wantAPI:  "apps/v1",
		},
		{
			name: "cluster scoped resource",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": "my-namespace",
						"uid":  "ns-123",
					},
				},
			},
			wantKind: "Namespace",
			wantName: "my-namespace",
			wantNS:   "",
			wantAPI:  "v1",
		},
		{
			name: "custom resource",
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kyverno.io/v1",
					"kind":       "ClusterPolicy",
					"metadata": map[string]interface{}{
						"name": "require-labels",
						"uid":  "pol-789",
					},
				},
			},
			wantKind: "ClusterPolicy",
			wantName: "require-labels",
			wantNS:   "",
			wantAPI:  "kyverno.io/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResourceSpecFromUnstructured(tt.obj)

			if result.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", result.Kind, tt.wantKind)
			}
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if result.Namespace != tt.wantNS {
				t.Errorf("Namespace = %q, want %q", result.Namespace, tt.wantNS)
			}
			if result.APIVersion != tt.wantAPI {
				t.Errorf("APIVersion = %q, want %q", result.APIVersion, tt.wantAPI)
			}
		})
	}
}

func TestResourceSpecFromUnstructured_HasUID(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
				"uid":       "uid-123-456",
			},
		},
	}

	result := ResourceSpecFromUnstructured(obj)

	if result.UID != "uid-123-456" {
		t.Errorf("UID = %q, want %q", result.UID, "uid-123-456")
	}
}

func TestResourceSpecFromUnstructured_ReturnsResourceSpec(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "my-secret",
				"namespace": "kyverno",
			},
		},
	}

	result := ResourceSpecFromUnstructured(obj)

	// Verify the result is of type ResourceSpec
	var _ kyverno.ResourceSpec = result
}

func TestResourceSpecFromUnstructured_EmptyObject(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	result := ResourceSpecFromUnstructured(obj)

	if result.Kind != "" {
		t.Errorf("Kind should be empty for empty object, got %q", result.Kind)
	}
	if result.Name != "" {
		t.Errorf("Name should be empty for empty object, got %q", result.Name)
	}
}

func TestUpdateStatus_ExceedsRetryLimit_DeletesWithoutUpdatingStatus(t *testing.T) {
	ur := &kyvernov2.UpdateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failed-ur",
			Namespace: config.KyvernoNamespace(),
		},
		Status: kyvernov2.UpdateRequestStatus{
			RetryCount: 4, // > 3, so retryOrDeleteOnFailure deletes it
			State:      kyvernov2.Pending,
		},
	}
	client := fake.NewSimpleClientset(ur)

	_, err := UpdateStatus(context.Background(), client, nil, ur.GetName(), kyvernov2.Failed, "permanent failure", nil)
	assert.NoError(t, err, "exceeding retry limit should delete UR cleanly without attempting an UpdateStatus on deleted object")

	// Verify the UR was indeed deleted
	_, getErr := client.KyvernoV2().UpdateRequests(config.KyvernoNamespace()).Get(context.Background(), ur.GetName(), metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(getErr), "expected UR to be deleted from store")
}

