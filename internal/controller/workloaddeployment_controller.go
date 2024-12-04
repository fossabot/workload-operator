// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkingv1alpha "go.datum.net/network-services-operator/api/v1alpha"
	computev1alpha "go.datum.net/workload-operator/api/v1alpha"
)

// WorkloadDeploymentReconciler reconciles a WorkloadDeployment object
type WorkloadDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=compute.datumapis.com,resources=workloaddeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=compute.datumapis.com,resources=workloaddeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=compute.datumapis.com,resources=workloaddeployments/finalizers,verbs=update

func (r *WorkloadDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var deployment computev1alpha.WorkloadDeployment
	if err := r.Client.Get(ctx, req.NamespacedName, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !deployment.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	logger.Info("reconciling deployment")
	defer logger.Info("reconcile complete")

	// Ensure that a `NetworkBinding` is created for each network interface's
	// network.

	if deployment.Status.ClusterRef == nil {
		return ctrl.Result{}, nil
	}

	// TODO(jreese) shortcut work on a status condition for network bindings
	// being ready

	for i, networkInterface := range deployment.Spec.Template.Spec.NetworkInterfaces {
		var networkBinding networkingv1alpha.NetworkBinding
		networkBindingObjectKey := client.ObjectKey{
			Namespace: deployment.Namespace,
			Name:      fmt.Sprintf("%s-net-%d", deployment.Name, i),
		}

		if err := r.Client.Get(ctx, networkBindingObjectKey, &networkBinding); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed checking for existing network binding: %w", err)
		}

		if networkBinding.CreationTimestamp.IsZero() {
			clusterRef := deployment.Status.ClusterRef
			networkBinding = networkingv1alpha.NetworkBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: networkBindingObjectKey.Namespace,
					Name:      networkBindingObjectKey.Name,
				},
				Spec: networkingv1alpha.NetworkBindingSpec{
					Network: networkInterface.Network,
					Topology: map[string]string{
						// TODO(jreese) move to well known labels package
						"topology.datum.net/cluster-namespace": clusterRef.Namespace,
						"topology.datum.net/cluster-name":      clusterRef.Name,
						"topology.datum.net/city-code":         deployment.Spec.CityCode,
					},
				},
			}

			if err := controllerutil.SetControllerReference(&deployment, &networkBinding, r.Scheme); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to set controller on network binding: %w", err)
			}

			if err := r.Client.Create(ctx, &networkBinding); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed creating network binding: %w", err)
			}
		}

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// TODO(jreese) finalizers
	return ctrl.NewControllerManagedBy(mgr).
		For(&computev1alpha.WorkloadDeployment{}).
		Complete(r)
}
