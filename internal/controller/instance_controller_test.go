package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	computev1alpha "go.datum.net/workload-operator/api/v1alpha"
)

type testManager struct {
	ctrl.Manager
	client client.Client
	scheme *runtime.Scheme
}

func (m *testManager) GetClient() client.Client {
	return m.client
}

func (m *testManager) GetScheme() *runtime.Scheme {
	return m.scheme
}

func TestInstanceReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(computev1alpha.AddToScheme(scheme))

	tests := []struct {
		name           string
		objs           []client.Object
		req            ctrl.Request
		expectedErr    string
		expectedLabels map[string]string
	}{
		{
			name: "missing controller owner in instance",
			objs: []client.Object{
				&computev1alpha.Instance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "instance-no-owner",
						Namespace: "default",
					},
				},
			},
			req:         ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "instance-no-owner"}},
			expectedErr: "failed to get controller owner of Instance",
		},
		{
			name: "workload deployment not found",
			objs: []client.Object{
				&computev1alpha.Instance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "instance-missing-wd",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "compute.datumapis.com/v1alpha",
								Kind:       "WorkloadDeployment",
								Name:       "wd1",
								UID:        "33c325cb-3f2e-4b2a-be0c-6d7e03aa475a",
								Controller: proto.Bool(true),
							},
						},
					},
				},
			},
			req:         ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "instance-missing-wd"}},
			expectedErr: "not found",
		},
		{
			name: "missing controller owner in workload deployment",
			objs: []client.Object{
				&computev1alpha.Instance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "instance-wd-no-owner",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "compute.datumapis.com/v1alpha",
								Kind:       "WorkloadDeployment",
								Name:       "wd1",
								UID:        "33c325cb-3f2e-4b2a-be0c-6d7e03aa475a",
								Controller: proto.Bool(true),
							},
						},
					},
				},
				&computev1alpha.WorkloadDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wd1",
						Namespace: "default",
						// Missing controller owner
					},
				},
			},
			req:         ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "instance-wd-no-owner"}},
			expectedErr: "failed to get controller owner of WorkloadDeployment",
		},
		{
			name: "successful reconcile and update",
			objs: []client.Object{
				&computev1alpha.Instance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "instance-success",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "compute.datumapis.com/v1alpha",
								Kind:       "WorkloadDeployment",
								Name:       "wd1",
								UID:        "33c325cb-3f2e-4b2a-be0c-6d7e03aa475a",
								Controller: proto.Bool(true),
							},
						},
					},
				},
				&computev1alpha.WorkloadDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wd1",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "compute.datumapis.com/v1alpha",
								Kind:       "Workload",
								Name:       "w1",
								UID:        "2561b624-3f7d-49db-bc74-b9dc71c4c08d",
								Controller: proto.Bool(true),
							},
						},
					},
				},
			},
			req:         ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "instance-success"}},
			expectedErr: "",
			expectedLabels: map[string]string{
				computev1alpha.WorkloadUIDLabel:           "2561b624-3f7d-49db-bc74-b9dc71c4c08d",
				computev1alpha.WorkloadDeploymentUIDLabel: "33c325cb-3f2e-4b2a-be0c-6d7e03aa475a",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(tc.objs...).Build()

			testMgr := &testManager{
				client: cl,
				scheme: scheme,
			}

			mgr, err := mcmanager.WithMultiCluster(testMgr, nil)
			if err != nil {
				t.Fatalf("failed to create manager: %v", err)
			}

			reconciler := &InstanceReconciler{mgr}
			_, err = reconciler.Reconcile(context.Background(), mcreconcile.Request{
				Request:     tc.req,
				ClusterName: "",
			})

			// Check error
			if tc.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.expectedErr)
			}

			// If labels are expected, fetch the instance and validate the labels.
			if tc.expectedLabels != nil {
				instance := &computev1alpha.Instance{}
				if err := cl.Get(context.Background(), tc.req.NamespacedName, instance); err != nil {
					t.Fatalf("failed to get instance: %v", err)
				}
				assert.Equal(t, tc.expectedLabels, instance.Labels)
			}
		})
	}
}
