package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const annotationEnabled = "idle-scale.nous.io/enabled"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	conntrackPath := "/host/proc/net/nf_conntrack"
	if p := os.Getenv("CONNTRACK_PATH"); p != "" {
		conntrackPath = p
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		if err != nil {
			klog.Fatalf("Failed to get k8s config: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create k8s client: %v", err)
	}

	tracker := newServiceTracker(clientset)
	go tracker.syncLoop(ctx)

	conntrackWatcher(ctx, conntrackPath, tracker)
}

func conntrackWatcher(ctx context.Context, path string, tracker *serviceTracker) {
	klog.Infof("Watching conntrack: %s", path)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		data, err := os.ReadFile(path)
		if err != nil {
			klog.Errorf("read conntrack: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		tracker.mu.RLock()
		for clusterIP := range tracker.ipToDeploy {
			needle := fmt.Sprintf(" dst=%s ", clusterIP)
			if strings.Contains(string(data), needle) {
				klog.Infof("traffic for %s -> scaling %s/%s",
					clusterIP, tracker.ipToDeploy[clusterIP].namespace, tracker.ipToDeploy[clusterIP].name)
				go tracker.scaleUp(ctx, clusterIP)
			}
		}
		tracker.mu.RUnlock()
		time.Sleep(2 * time.Second)
	}
}

type deployRef struct {
	namespace string
	name      string
}

type serviceTracker struct {
	clientset  kubernetes.Interface
	ipToDeploy map[string]deployRef
	mu         sync.RWMutex
}

func newServiceTracker(c kubernetes.Interface) *serviceTracker {
	return &serviceTracker{
		clientset:  c,
		ipToDeploy: map[string]deployRef{},
	}
}

func (t *serviceTracker) syncLoop(ctx context.Context) {
	t.sync(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.sync(ctx)
		}
	}
}

func (t *serviceTracker) sync(ctx context.Context) {
	req, _ := labels.NewRequirement(annotationEnabled, selection.Equals, []string{"true"})
	svcs, err := t.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: req.String(),
	})
	if err != nil {
		klog.Errorf("list services: %v", err)
		return
	}
	newMap := map[string]deployRef{}
	for _, svc := range svcs.Items {
		if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
			continue
		}
		sel := labels.SelectorFromSet(svc.Spec.Selector)
		deploys, err := t.clientset.AppsV1().Deployments(svc.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: sel.String(),
		})
		if err != nil || len(deploys.Items) == 0 {
			continue
		}
		deploy := deploys.Items[0]
		if deploy.Annotations == nil || deploy.Annotations[annotationEnabled] != "true" {
			continue
		}
		newMap[svc.Spec.ClusterIP] = deployRef{
			namespace: deploy.Namespace,
			name:      deploy.Name,
		}
	}
	t.mu.Lock()
	t.ipToDeploy = newMap
	t.mu.Unlock()
	klog.V(2).Infof("tracked %d idle services", len(newMap))
}

func (t *serviceTracker) scaleUp(ctx context.Context, clusterIP string) {
	t.mu.RLock()
	ref, ok := t.ipToDeploy[clusterIP]
	t.mu.RUnlock()
	if !ok {
		return
	}
	deploy, err := t.clientset.AppsV1().Deployments(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("get deploy %s/%s: %v", ref.namespace, ref.name, err)
		return
	}
	if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas > 0 {
		return
	}
	one := int32(1)
	deploy.Spec.Replicas = &one
	if _, err := t.clientset.AppsV1().Deployments(ref.namespace).Update(ctx, deploy, metav1.UpdateOptions{}); err != nil {
		klog.Errorf("scale up %s/%s: %v", ref.namespace, ref.name, err)
		return
	}
	klog.Infof("scaled up %s/%s (conntrack)", ref.namespace, ref.name)
}
