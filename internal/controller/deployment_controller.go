package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	idlev1 "github.com/EzzOps/idle-scale/internal/common"
)

type DeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config Config
}

type Config struct {
	SentinelImage string
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments/scale,verbs=get;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
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

	// Handle expired sentinels (exit code 42 = traffic detected)
	if err := r.handleTerminatedSentinels(ctx, &deploy); err != nil {
		log.Error(err, "failed to handle terminated sentinels")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if rep > 0 {
		// Active: cleanup any leftover sentinels
		if err := r.cleanupSentinels(ctx, &deploy); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// At 0 replicas: ensure a sentinel is running
	active, err := r.hasActiveSentinel(ctx, &deploy)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !active {
		log.Info("creating sentinel pod", "deployment", deploy.Name)
		if err := r.createSentinel(ctx, &deploy); err != nil {
			return ctrl.Result{}, fmt.Errorf("create sentinel: %w", err)
		}
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *DeploymentReconciler) handleTerminatedSentinels(ctx context.Context, deploy *appsv1.Deployment) error {
	log := logf.FromContext(ctx)
	list, err := r.listSentinels(ctx, deploy)
	if err != nil {
		return err
	}
	for _, pod := range list {
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			continue
		}
		cs := pod.Status.ContainerStatuses
		if len(cs) > 0 && cs[0].State.Terminated != nil && cs[0].State.Terminated.ExitCode == idlev1.SentinelExitCode {
			log.Info("traffic detected via sentinel, scaling up")
			patch := client.MergeFrom(deploy.DeepCopy())
			deploy.Spec.Replicas = ptr(int32(1))
			if err := r.Patch(ctx, deploy, patch); err != nil {
				return err
			}
		}
		if err := r.Delete(ctx, &pod); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func (r *DeploymentReconciler) createSentinel(ctx context.Context, deploy *appsv1.Deployment) error {
	name := fmt.Sprintf("%s-sentinel", deploy.Name)
	labels := deploy.Spec.Template.ObjectMeta.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels[idlev1.LabelSentinel] = "sentinel"
	labels[idlev1.LabelDeployRef] = deploy.Name

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: deploy.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:            "sentinel",
				Image:           r.sentinelImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Args:            []string{"--port=" + r.discoverPort(deploy), "--ignore=" + r.discoverIgnorePaths(deploy)},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("5m"),
						corev1.ResourceMemory: resource.MustParse("10Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("20Mi"),
					},
				},
			}},
			RestartPolicy:                 corev1.RestartPolicyNever,
			TerminationGracePeriodSeconds: ptr(int64(2)),
		},
	}
	if err := controllerutil.SetControllerReference(deploy, pod, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, pod)
}

func (r *DeploymentReconciler) cleanupSentinels(ctx context.Context, deploy *appsv1.Deployment) error {
	list, err := r.listSentinels(ctx, deploy)
	if err != nil {
		return err
	}
	for _, pod := range list {
		if err := r.Delete(ctx, &pod); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func (r *DeploymentReconciler) hasActiveSentinel(ctx context.Context, deploy *appsv1.Deployment) (bool, error) {
	list, err := r.listSentinels(ctx, deploy)
	if err != nil {
		return false, err
	}
	for _, pod := range list {
		if pod.Status.Phase == corev1.PodRunning {
			return true, nil
		}
	}
	return false, nil
}

func (r *DeploymentReconciler) listSentinels(ctx context.Context, deploy *appsv1.Deployment) ([]corev1.Pod, error) {
	list := &corev1.PodList{}
	if err := r.List(ctx, list,
		client.InNamespace(deploy.Namespace),
		client.MatchingLabels{
			idlev1.LabelSentinel:  "sentinel",
			idlev1.LabelDeployRef: deploy.Name,
		},
	); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *DeploymentReconciler) discoverPort(deploy *appsv1.Deployment) string {
	if p, ok := deploy.Annotations[idlev1.AnnotationPort]; ok {
		return p
	}
	return "8080"
}

func (r *DeploymentReconciler) discoverIgnorePaths(deploy *appsv1.Deployment) string {
	if p, ok := deploy.Annotations[idlev1.AnnotationIgnorePaths]; ok {
		return p
	}
	return "/healthz,/readyz,/livez,/metrics"
}

func (r *DeploymentReconciler) sentinelImage() string {
	if img := os.Getenv("SENTINEL_IMAGE"); img != "" {
		return img
	}
	return "ghcr.io/ezzops/idle-scale-sentinel:latest"
}

func ptr[T any](v T) *T { return &v }

func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		Owns(&corev1.Pod{}).
		Named("idle-scale").
		Complete(r)
}
