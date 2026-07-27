package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	idlev1 "github.com/EzzOps/idle-scale/internal/common"
)

type DeploymentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments/scale,verbs=get;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("deployment", req.NamespacedName)

	var deploy appsv1.Deployment
	if err := r.Get(ctx, req.NamespacedName, &deploy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if deploy.Annotations == nil || deploy.Annotations[idlev1.AnnotationEnabled] != "true" {
		return ctrl.Result{}, nil
	}

	rep := int32(1)
	if deploy.Spec.Replicas != nil {
		rep = *deploy.Spec.Replicas
	}

	if rep == 0 {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if rep > 0 && r.isIdle(ctx, &deploy) {
		log.Info("idle timeout reached, scaling to zero")
		patch := client.MergeFrom(deploy.DeepCopy())
		zero := int32(0)
		(&deploy).Spec.Replicas = &zero
		if err := r.Patch(ctx, &deploy, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("scale to zero: %w", err)
		}
		r.Recorder.Event(&deploy, corev1.EventTypeNormal, "ScaledToZero",
			"Deployment scaled to zero (idle timeout)")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *DeploymentReconciler) isIdle(ctx context.Context, deploy *appsv1.Deployment) bool {
	if deploy.Status.AvailableReplicas > 0 || deploy.Status.ReadyReplicas > 0 {
		return false
	}

	grace := parseDuration(deploy.Annotations[idlev1.AnnotationStartupGrace], "10m")
	if time.Since(deploy.CreationTimestamp.Time) < grace {
		return false
	}

	timeout := parseDuration(deploy.Annotations[idlev1.AnnotationIdleTimeout], "10m")
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(deploy.Namespace),
		client.MatchingLabels(deploy.Spec.Selector.MatchLabels)); err != nil {
		return false
	}
	for _, pod := range podList.Items {
		if time.Since(pod.CreationTimestamp.Time) < timeout {
			return false
		}
	}
	return true
}

func parseDuration(s, def string) time.Duration {
	if s != "" {
		d, err := time.ParseDuration(s)
		if err == nil {
			return d
		}
	}
	d, _ := time.ParseDuration(def)
	return d
}

func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		Named("idle-scale").
		Complete(r)
}
