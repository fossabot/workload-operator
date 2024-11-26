// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterinventoryv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	computev1alpha "go.datum.net/workload-operator/api/v1alpha"
)

// WorkloadDeploymentScheduler schedules a WorkloadDeployment
type WorkloadDeploymentScheduler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *WorkloadDeploymentScheduler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	logger.Info("scheduling deployment")
	defer logger.Info("scheduling complete")

	// TODO(jreese) improve!
	// The first iteration of this scheduler will be very simple and only look for
	// the first available cluster that is viable for the deployment. In the
	// future, we could see a more advanced system similar to the Kubernetes
	// scheduler itself.

	// Step 1: Get ClusterProfiles
	var clusterProfiles clusterinventoryv1alpha1.ClusterProfileList
	if err := r.Client.List(ctx, &clusterProfiles); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list cluster profiles: %w", err)
	}

	if len(clusterProfiles.Items) == 0 {
		// Should only be the case in new environments if workloads are created
		// prior to cluster registration.

		changed := apimeta.SetStatusCondition(&deployment.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "NoClusterProfiles",
			ObservedGeneration: deployment.Generation,
			Message:            "No cluster profiles are registered with the system.",
		})
		if changed {
			// TODO(jreese) investigate kubevirt / other operators for better tracking
			// of updates to the status. I seem to remember a "builder" of sorts that
			// looked rather nice.
			if err := r.Client.Status().Update(ctx, &deployment); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update deployment status: %w", err)
			}
		}

		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// TODO(jreese) define standard ClusterProperty names somewhere

	var selectedCluster *clusterinventoryv1alpha1.ClusterProfile
	for _, clusterProfile := range clusterProfiles.Items {
		for _, p := range clusterProfile.Status.Properties {
			if p.Name == "cityCode" && p.Value == deployment.Spec.CityCode {
				selectedCluster = &clusterProfile
				break
			}
		}
	}

	if selectedCluster == nil {
		changed := apimeta.SetStatusCondition(&deployment.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "NoCandidateClusters",
			ObservedGeneration: deployment.Generation,
			Message:            "No clusters are candidates for this deployment.",
		})
		if changed {
			if err := r.Client.Status().Update(ctx, &deployment); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update deployment status: %w", err)
			}
		}
	} else {
		deployment.Status.ClusterProfileRef = &computev1alpha.ClusterProfileReference{
			Name:      selectedCluster.Name,
			Namespace: selectedCluster.Namespace,
		}

		// TODO(jreese) make sure we don't run into update conflicts with the update
		// of the spec then status here. Just can't remember if it's an issue.

		apimeta.SetStatusCondition(&deployment.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "ClusterAssigned",
			ObservedGeneration: deployment.Generation,
			Message:            "Deployment has been assigned a cluster.",
		})

		if err := r.Client.Status().Update(ctx, &deployment); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update deployment status: %w", err)
		}

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadDeploymentScheduler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&computev1alpha.WorkloadDeployment{}, builder.WithPredicates(
			predicate.NewPredicateFuncs(func(object client.Object) bool {
				// Don't bother processing deployments that have been scheduled
				o := object.(*computev1alpha.WorkloadDeployment)
				return o.Status.ClusterProfileRef == nil
			}),
		)).
		Named("workload-deployment-scheduler").
		Complete(r)
}
