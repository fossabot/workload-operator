// SPDX-License-Identifier: AGPL-3.0-only

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mccontext "sigs.k8s.io/multicluster-runtime/pkg/context"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"

	computev1alpha "go.datum.net/workload-operator/api/v1alpha"
)

// InstanceReconciler reconciles an Instance object
type InstanceReconciler struct {
	mgr mcmanager.Manager
}

// +kubebuilder:rbac:groups=compute.datumapis.com,resources=instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=compute.datumapis.com,resources=instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=compute.datumapis.com,resources=instances/finalizers,verbs=update

func (r *InstanceReconciler) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cl, err := r.mgr.GetCluster(ctx, req.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	ctx = mccontext.WithCluster(ctx, req.ClusterName)
	var instance computev1alpha.Instance
	if err := cl.GetClient().Get(ctx, req.NamespacedName, &instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	logger.Info("reconciling instance")
	defer logger.Info("reconcile complete")

	// Ensure the instance has labels necessary for being able to identify
	// instances associated with a specific workload or workload deployment via
	// label selectors.
	//
	// This logic will not be necessary once we complete the work defined in
	// https://github.com/datum-cloud/enhancements/issues/28

	workloadDeploymentRef := metav1.GetControllerOf(&instance)
	if workloadDeploymentRef == nil {
		return ctrl.Result{}, fmt.Errorf("failed to get controller owner of Instance")
	}

	var workloadDeployment computev1alpha.WorkloadDeployment
	workloadDeploymentObjectKey := client.ObjectKey{
		Namespace: instance.Namespace,
		Name:      workloadDeploymentRef.Name,
	}
	if err := cl.GetClient().Get(ctx, workloadDeploymentObjectKey, &workloadDeployment); err != nil {
		return ctrl.Result{}, err
	}

	workloadRef := metav1.GetControllerOf(&workloadDeployment)
	if workloadRef == nil {
		return ctrl.Result{}, fmt.Errorf("failed to get controller owner of WorkloadDeployment")
	}

	updated := instance.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	updated.Labels[computev1alpha.WorkloadUIDLabel] = string(workloadRef.UID)
	updated.Labels[computev1alpha.WorkloadDeploymentUIDLabel] = string(workloadDeploymentRef.UID)

	if !equality.Semantic.DeepEqual(updated, instance) {
		if err := cl.GetClient().Update(ctx, updated); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed updating instance: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstanceReconciler) SetupWithManager(mgr mcmanager.Manager) error {
	r.mgr = mgr
	return mcbuilder.ControllerManagedBy(mgr).
		For(&computev1alpha.Instance{}, mcbuilder.WithEngageWithLocalCluster(false)).
		Complete(r)
}
