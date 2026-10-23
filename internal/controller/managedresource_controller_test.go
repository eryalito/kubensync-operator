/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/event"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
	"github.com/eryalito/kubensync-operator/internal/watch"
)

// newReconciler returns a reconciler that uses c for the ManagedResource and cfg for everything else.
func newReconciler(c client.Client) *ManagedResourceReconciler {
	r := &ManagedResourceReconciler{
		Client: c, apiReader: c, discoveryClient: discovery.NewDiscoveryClientForConfigOrDie(cfg),
		watches: startWatchManager(func(context.Context, watch.Key, func()) (func() bool, error) { return func() bool { return true }, nil }),
	}
	setConfig(r, cfg)
	return r
}

func startWatchManager(start watch.StartFunc) *watch.Manager {
	m := watch.NewManager(start, time.Second)
	go func() { _ = m.Start(ctx) }()
	return m
}

func newTestManager() ctrl.Manager {
	skipNameValidation := true
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             config.Controller{SkipNameValidation: &skipNameValidation},
	})
	Expect(err).NotTo(HaveOccurred())
	return mgr
}

// runManagedResourceController runs a ManagedResource controller until the spec ends.
func runManagedResourceController() {
	mgr := newTestManager()
	Expect((&ManagedResourceReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr)).To(Succeed())
	mgrCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		defer close(done)
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()
	DeferCleanup(func() {
		stop()
		<-done
	})
}

func configMapValue(namespace, name, key string) func() string {
	return func() string {
		cm := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err != nil {
			return ""
		}
		return cm.Data[key]
	}
}

// setConfig points the reconciler's requests for templated resources at c.
func setConfig(r *ManagedResourceReconciler, c *rest.Config) {
	r.config = c
	r.dynamicClient = dynamic.NewForConfigOrDie(c)
}

// newNamespaceController returns a namespace controller that shares r's clients.
func newNamespaceController(r *ManagedResourceReconciler) *NamespaceController {
	return &NamespaceController{
		Client: k8sClient, apiReader: k8sClient, config: r.config,
		discoveryClient: r.discoveryClient, dynamicClient: r.dynamicClient,
	}
}

// newCountingReconciler returns a reconciler whose status writes are counted.
func newCountingReconciler() (*ManagedResourceReconciler, *int) {
	statusWrites := 0
	base, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	c := interceptor.NewClient(base, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			statusWrites++
			return c.SubResource(subResource).Update(ctx, obj, opts...)
		},
		SubResourcePatch: func(ctx context.Context, c client.Client, subResource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			statusWrites++
			return c.SubResource(subResource).Patch(ctx, obj, patch, opts...)
		},
	})
	return newReconciler(c), &statusWrites
}

// createManagedResource creates count namespaces named prefix-N and a ManagedResource
// selecting them that renders literal, and returns the request to reconcile it.
func createManagedResource(prefix string, count int, literal string) (*automationv1alpha1.ManagedResource, ctrl.Request) {
	for i := range count {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%d", prefix, i)}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
	}
	mr := &automationv1alpha1.ManagedResource{
		ObjectMeta: metav1.ObjectMeta{Name: prefix},
		Spec: automationv1alpha1.ManagedResourceSpec{
			NamespaceSelector: automationv1alpha1.ManagedResourceSpecNamespaceSelector{Regex: "^" + prefix + "-"},
			Template:          automationv1alpha1.ManagedResourceSpecTemplate{Literal: literal},
		},
	}
	Expect(k8sClient.Create(ctx, mr)).To(Succeed())
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, mr))).To(Succeed())
	})
	return mr, ctrl.Request{NamespacedName: types.NamespacedName{Name: mr.Name}}
}

func configMapLiteral(name, value string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: {{ .Namespace.Name }}
data:
  value: %s
`, name, value)
}

// newReconcilerWithStatusHook returns a reconciler that runs hook once, right before its
// first status write reaches the API server.
func newReconcilerWithStatusHook(hook func(name string)) *ManagedResourceReconciler {
	called := false
	base, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	c := interceptor.NewClient(base, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if !called {
				called = true
				hook(obj.GetName())
			}
			return c.SubResource(subResource).Update(ctx, obj, opts...)
		},
	})
	return newReconciler(c)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// recordConfigMapNamespaces makes r record the namespaces whose ConfigMaps it touches.
func recordConfigMapNamespaces(r *ManagedResourceReconciler) map[string]bool {
	touched := map[string]bool{}
	setConfig(r, configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/configmaps") {
			touched[strings.Split(strings.TrimPrefix(req.URL.Path, "/api/v1/namespaces/"), "/")[0]] = true
		}
		return next.RoundTrip(req)
	}))
	return touched
}

// configWithTransport returns a copy of cfg whose requests go through wrap.
func configWithTransport(wrap func(next http.RoundTripper, req *http.Request) (*http.Response, error)) *rest.Config {
	c := rest.CopyConfig(cfg)
	c.Wrap(func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(req *http.Request) (*http.Response, error) { return wrap(next, req) })
	})
	return c
}

// newCRD returns a namespaced CRD for kind in group, served as v1.
func newCRD(group, kind, plural string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]interface{}{"name": plural + "." + group},
		"spec": map[string]interface{}{
			"group": group,
			"scope": "Namespaced",
			"names": map[string]interface{}{"plural": plural, "singular": strings.ToLower(kind), "kind": kind, "listKind": kind + "List"},
			"versions": []interface{}{map[string]interface{}{
				"name": "v1", "served": true, "storage": true,
				"schema": map[string]interface{}{"openAPIV3Schema": map[string]interface{}{
					"type": "object", "x-kubernetes-preserve-unknown-fields": true,
				}},
			}},
		},
	}}
}

var _ = Describe("ManagedResource controller", func() {
	BeforeEach(func() {
		previous := statusCheckpointInterval
		statusCheckpointInterval = time.Hour
		DeferCleanup(func() { statusCheckpointInterval = previous })
	})

	It("writes status only when it changes", func() {
		r, statusWrites := newCountingReconciler()
		mr, req := createManagedResource("status-write", 3, configMapLiteral("status-write", "ok"))

		By("creating the resources in every matching namespace")
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		// Once after the namespace pass, once with the Ready condition.
		Expect(*statusWrites).To(Equal(2))
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(3))
		Expect(apimeta.IsStatusConditionTrue(mr.Status.Conditions, automationv1alpha1.ConditionReady)).To(BeTrue())

		By("reconciling again without changes")
		*statusWrites = 0
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(*statusWrites).To(BeZero())

		By("removing the resources of a namespace that no longer matches")
		mr.Spec.NamespaceSelector.Regex = "^status-write-[01]$"
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		*statusWrites = 0
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(*statusWrites).To(Equal(2))
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(2))
		cm := &corev1.ConfigMap{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "status-write-2", Name: "status-write"}, cm)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("keeps tracking resources of earlier namespaces when a later namespace fails to render", func() {
		r, _ := newCountingReconciler()
		literal := configMapLiteral("partial", `{{ if eq .Namespace.Name "partial-2" }}{{ .Namespace.Nope }}{{ else }}ok{{ end }}`)
		mr, req := createManagedResource("partial", 3, literal)

		_, err := r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(2))
		Expect(apimeta.IsStatusConditionFalse(mr.Status.Conditions, automationv1alpha1.ConditionReady)).To(BeTrue())
	})

	It("keeps tracking resources applied before a failure inside a namespace", func() {
		r, _ := newCountingReconciler()
		literal := configMapLiteral("applied", "ok") + `{{ if eq .Namespace.Name "applied-2" }}---
apiVersion: example.com/v1
kind: DoesNotExist
metadata:
  name: missing
  namespace: {{ .Namespace.Name }}
{{ end }}`
		mr, req := createManagedResource("applied", 3, literal)

		_, err := r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(3))
		Expect(apimeta.IsStatusConditionFalse(mr.Status.Conditions, automationv1alpha1.ConditionReady)).To(BeTrue())
	})

	It("writes status over a spec edit made during the reconcile without mixing specs in the pass", func() {
		statusCheckpointInterval = time.Nanosecond
		r := newReconcilerWithStatusHook(func(name string) {
			mr := &automationv1alpha1.ManagedResource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, mr)).To(Succeed())
			mr.Spec.Template.Literal = configMapLiteral("edited", "new")
			Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		})
		mr, req := createManagedResource("edited", 3, configMapLiteral("edited", "old"))
		generation := mr.Generation

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(3))
		ready := apimeta.FindStatusCondition(mr.Status.Conditions, automationv1alpha1.ConditionReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.ObservedGeneration).To(Equal(generation))
		for i := range 3 {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: fmt.Sprintf("edited-%d", i), Name: "edited"}, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("value", "old"))
		}
	})

	It("does not overwrite status written concurrently by another writer", func() {
		foreign := automationv1alpha1.CreatedResource{
			ApiVersion: "v1", Kind: "ConfigMap", Name: "foreign", Namespace: "default", UID: "foreign", TriggerNamespace: "default",
		}
		r := newReconcilerWithStatusHook(func(name string) {
			mr := &automationv1alpha1.ManagedResource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, mr)).To(Succeed())
			mr.Status.CreatedResources = append(mr.Status.CreatedResources, foreign)
			Expect(k8sClient.Status().Update(ctx, mr)).To(Succeed())
		})
		mr, req := createManagedResource("foreign-writer", 1, configMapLiteral("foreign-writer", "ok"))

		_, err := r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(ContainElement(foreign))
	})

	It("stops the pass when the ManagedResource is deleted mid-pass", func() {
		statusCheckpointInterval = time.Nanosecond
		r := newReconcilerWithStatusHook(func(name string) {
			Expect(k8sClient.Delete(ctx, &automationv1alpha1.ManagedResource{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
		})
		touched := recordConfigMapNamespaces(r)
		_, req := createManagedResource("deleted", 3, configMapLiteral("deleted", "ok"))

		_, err := r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(touched).To(HaveKey("deleted-0"))
		Expect(touched).NotTo(HaveKey("deleted-1"))
		Expect(touched).NotTo(HaveKey("deleted-2"))
	})

	It("stops the pass when the ManagedResource starts being deleted mid-pass", func() {
		statusCheckpointInterval = time.Nanosecond
		r := newReconcilerWithStatusHook(func(name string) {
			mr := &automationv1alpha1.ManagedResource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, mr)).To(Succeed())
			mr.Finalizers = append(mr.Finalizers, "test.kubensync.com/hold")
			Expect(k8sClient.Update(ctx, mr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, mr)).To(Succeed())
		})
		touched := recordConfigMapNamespaces(r)
		_, req := createManagedResource("deleting", 3, configMapLiteral("deleting", "ok"))
		DeferCleanup(func() {
			mr := &automationv1alpha1.ManagedResource{}
			if k8sClient.Get(ctx, req.NamespacedName, mr) == nil {
				mr.Finalizers = nil
				Expect(k8sClient.Update(ctx, mr)).To(Succeed())
			}
		})

		_, err := r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(touched).To(HaveKey("deleting-0"))
		Expect(touched).NotTo(HaveKey("deleting-1"))
		Expect(touched).NotTo(HaveKey("deleting-2"))
	})

	It("records created resources when the reconcile is cancelled mid-pass", func() {
		r, _ := newCountingReconciler()
		reconcileCtx, cancelReconcile := context.WithCancel(ctx)
		DeferCleanup(cancelReconcile)
		creates := 0
		setConfig(r, configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
			if creates == 3 {
				cancelReconcile()
			}
			resp, err := next.RoundTrip(req)
			if err == nil && req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/configmaps") {
				creates++
			}
			return resp, err
		}))
		mr, req := createManagedResource("cancelled", 4, configMapLiteral("cancelled", "ok"))

		_, err := r.Reconcile(reconcileCtx, req)
		Expect(err).To(HaveOccurred())
		Expect(creates).To(Equal(3))
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(3))
	})

	It("reports conflicts on managed resources as not ready", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("target-conflict", 1, configMapLiteral("target-conflict", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("changing the template so the next reconcile has to update the ConfigMap")
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		mr.Spec.Template.Literal = configMapLiteral("target-conflict", "changed")
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		setConfig(r, configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPut || !strings.Contains(req.URL.Path, "/configmaps/") {
				return next.RoundTrip(req)
			}
			body := `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Conflict","code":409,"message":"injected conflict"}`
			return &http.Response{
				StatusCode: http.StatusConflict,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}))

		_, err = r.Reconcile(ctx, req)
		Expect(apierrors.IsConflict(err)).To(BeTrue())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(apimeta.IsStatusConditionFalse(mr.Status.Conditions, automationv1alpha1.ConditionReady)).To(BeTrue())
	})

	It("checkpoints status during the namespace pass", func() {
		statusCheckpointInterval = time.Nanosecond
		r, statusWrites := newCountingReconciler()
		mr, req := createManagedResource("checkpoint", 3, configMapLiteral("checkpoint", "ok"))

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		// One checkpoint per namespace, then the Ready condition.
		Expect(*statusWrites).To(Equal(4))
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(3))
	})

	It("does not rewrite status when the template renders nothing", func() {
		r, statusWrites := newCountingReconciler()
		_, req := createManagedResource("empty-render", 1, "")

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(*statusWrites).To(Equal(1))

		*statusWrites = 0
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(*statusWrites).To(BeZero())
	})

	It("takes over objects left by a deleted ManagedResource with the same name", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("recreated", 1, configMapLiteral("recreated", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("recreating the ManagedResource before its objects are garbage collected")
		Expect(k8sClient.Delete(ctx, mr)).To(Succeed())
		recreated := &automationv1alpha1.ManagedResource{ObjectMeta: metav1.ObjectMeta{Name: mr.Name}, Spec: mr.Spec}
		Expect(k8sClient.Create(ctx, recreated)).To(Succeed())
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "recreated-0", Name: "recreated"}, cm)).To(Succeed())
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].UID).To(Equal(recreated.UID))
	})

	It("resolves kinds from CRDs installed after the reconciler started", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("late-crd", 1, configMapLiteral("late-crd", "ok"))

		By("warming up the reconciler's RESTMapper before the CRD exists")
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		crd := newCRD("late.kubensync.com", "Widget", "widgets")
		Expect(k8sClient.Create(ctx, crd)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, crd))).To(Succeed()) })

		By("rendering a resource of the new kind")
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		mr.Spec.Template.Literal = `apiVersion: late.kubensync.com/v1
kind: Widget
metadata:
  name: late-crd
  namespace: {{ .Namespace.Name }}
`
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		Eventually(func() error {
			_, err := r.Reconcile(ctx, req)
			return err
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		widget := &unstructured.Unstructured{}
		widget.SetAPIVersion("late.kubensync.com/v1")
		widget.SetKind("Widget")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "late-crd-0", Name: "late-crd"}, widget)).To(Succeed())
	})

	It("fails instead of treating objects as absent when their kind stops being served", func() {
		r, _ := newCountingReconciler()
		crd := newCRD("stale.kubensync.com", "Gadget", "gadgets")
		Expect(k8sClient.Create(ctx, crd)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, crd))).To(Succeed()) })
		mr, req := createManagedResource("stale", 1, configMapLiteral("stale", `{{ if .Data.g }}present{{ else }}absent{{ end }}`))
		gadget := &unstructured.Unstructured{}
		gadget.SetAPIVersion("stale.kubensync.com/v1")
		gadget.SetKind("Gadget")
		gadget.SetNamespace("stale-0")
		gadget.SetName("g")
		Eventually(func() error { return client.IgnoreAlreadyExists(k8sClient.Create(ctx, gadget)) }).
			WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		mr.Spec.Template.Data = []automationv1alpha1.ManagedResourceSpecTemplateData{{
			Name: "g",
			Type: automationv1alpha1.KubernetesResource,
			Ref: automationv1alpha1.ManagedResourceSpecTemplateDataRef{
				Group: "stale.kubensync.com", ApiVersion: "v1", Kind: "Gadget", Name: "g", Namespace: "stale-0",
			},
		}}
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		cm := &corev1.ConfigMap{}
		cmKey := types.NamespacedName{Namespace: "stale-0", Name: "stale"}
		Expect(k8sClient.Get(ctx, cmKey, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("value", "present"))

		By("deleting the CRD")
		Expect(k8sClient.Delete(ctx, crd)).To(Succeed())
		dc := discovery.NewDiscoveryClientForConfigOrDie(cfg)
		Eventually(func() bool {
			_, err := dc.ServerResourcesForGroupVersion("stale.kubensync.com/v1")
			return apierrors.IsNotFound(err)
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(BeTrue())

		_, err = r.Reconcile(ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(k8sClient.Get(ctx, cmKey, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("value", "present"))
	})

	It("wires its clients in SetupWithManager", func() {
		mgr := newTestManager()
		mrController := &ManagedResourceReconciler{Client: mgr.GetClient()}
		Expect(mrController.SetupWithManager(mgr)).To(Succeed())
		go func() { _ = mrController.watches.Start(ctx) }()
		nsController := &NamespaceController{Client: mgr.GetClient()}
		Expect(nsController.SetupWithManager(mgr)).To(Succeed())
		mr, req := createManagedResource("wired", 1, configMapLiteral("wired", "ok"))

		_, err := mrController.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("handling a namespace created afterwards")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "wired-1"}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
		Expect(nsController.reconcileNamespace(ctx, ns)).To(Succeed())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(2))
	})

	It("fetches template data once per reconcile", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("data-once", 3, configMapLiteral("data-once", `{{ index .Data.src "greeting" | quote }}`))
		src := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "data-src", Namespace: "data-once-0"},
			Data:       map[string]string{"greeting": "hello"},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, src))).To(Succeed())
		mr.Spec.Template.Data = []automationv1alpha1.ManagedResourceSpecTemplateData{{
			Name: "src",
			Type: automationv1alpha1.ConfigMap,
			Ref:  automationv1alpha1.ManagedResourceSpecTemplateDataRef{Name: "data-src", Namespace: "data-once-0"},
		}}
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		dataReads := 0
		setConfig(r, configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && req.URL.Path == "/api/v1/namespaces/data-once-0/configmaps/data-src" {
				dataReads++
			}
			return next.RoundTrip(req)
		}))

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(dataReads).To(Equal(1))
		for i := range 3 {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: fmt.Sprintf("data-once-%d", i), Name: "data-once"}, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("value", "hello"))
		}
	})

	It("gives each namespace its own copy of the template data", func() {
		r, _ := newCountingReconciler()
		literal := configMapLiteral("data-copy", `{{ hasKey "marker" .Data.src | quote }}{{ $_ := set "marker" "x" .Data.src }}`)
		mr, req := createManagedResource("data-copy", 2, literal)
		src := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "data-src", Namespace: "data-copy-0"},
			Data:       map[string]string{"greeting": "hello"},
		}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, src))).To(Succeed())
		mr.Spec.Template.Data = []automationv1alpha1.ManagedResourceSpecTemplateData{{
			Name: "src",
			Type: automationv1alpha1.ConfigMap,
			Ref:  automationv1alpha1.ManagedResourceSpecTemplateDataRef{Name: "data-src", Namespace: "data-copy-0"},
		}}
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())

		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		for _, ns := range []string{"data-copy-0", "data-copy-1"} {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "data-copy"}, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("value", "false"), ns)
		}
	})

	It("keeps several ManagedResources apart when rendering a namespace", func() {
		for _, name := range []string{"multi-a", "multi-b"} {
			src := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name + "-src", Namespace: "default"}, Data: map[string]string{"v": "from-" + name}}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, src))).To(Succeed())
			mr := &automationv1alpha1.ManagedResource{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: automationv1alpha1.ManagedResourceSpec{
					NamespaceSelector: automationv1alpha1.ManagedResourceSpecNamespaceSelector{Regex: "^multi-mr$"},
					Template: automationv1alpha1.ManagedResourceSpecTemplate{
						Literal: configMapLiteral(name, `{{ index .Data.src "v" | quote }}`),
						Data: []automationv1alpha1.ManagedResourceSpecTemplateData{{
							Name: "src",
							Type: automationv1alpha1.ConfigMap,
							Ref:  automationv1alpha1.ManagedResourceSpecTemplateDataRef{Name: name + "-src", Namespace: "default"},
						}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, mr)).To(Succeed())
			DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, mr))).To(Succeed()) })
		}
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "multi-mr"}}
		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
		nsController := newNamespaceController(newReconciler(k8sClient))

		Expect(nsController.reconcileNamespace(ctx, ns)).To(Succeed())
		for _, name := range []string{"multi-a", "multi-b"} {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "multi-mr", Name: name}, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("value", "from-"+name), name)
		}
	})

	It("removes every tracked resource whose namespace stops matching", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("unmatched", 3, configMapLiteral("unmatched", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(3))

		By("narrowing the selector so that no namespace matches")
		mr.Spec.NamespaceSelector.Regex = "^unmatched-none$"
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(BeEmpty())
		for i := range 3 {
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: fmt.Sprintf("unmatched-%d", i), Name: "unmatched"}, &corev1.ConfigMap{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}
	})

	It("removes tracked resources whose trigger namespace no longer exists", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("vanished", 1, configMapLiteral("vanished", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		orphan := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "vanished-0"}}
		Expect(k8sClient.Create(ctx, orphan)).To(Succeed())

		By("tracking an existing and an already deleted resource of a missing namespace")
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		rendered := mr.Status.CreatedResources
		for _, name := range []string{"orphan", "already-gone"} {
			mr.Status.CreatedResources = append(mr.Status.CreatedResources, automationv1alpha1.CreatedResource{
				ApiVersion: "v1", Kind: "ConfigMap", Name: name, Namespace: "vanished-0", TriggerNamespace: "vanished-gone",
			})
		}
		Expect(k8sClient.Status().Update(ctx, mr)).To(Succeed())

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(Equal(rendered))
		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(orphan), &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("does not fetch tracked resources or their namespaces one by one", func() {
		r, _ := newCountingReconciler()
		_, req := createManagedResource("tracked-gets", 3, configMapLiteral("tracked-gets", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		configMapGets, namespaceGets := 0, 0
		setConfig(r, configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet {
				segments := strings.Split(strings.TrimPrefix(req.URL.Path, "/api/v1/namespaces/"), "/")
				switch {
				case strings.Contains(req.URL.Path, "/configmaps/"):
					configMapGets++
				case strings.HasPrefix(req.URL.Path, "/api/v1/namespaces/") && len(segments) == 1:
					namespaceGets++
				}
			}
			return next.RoundTrip(req)
		}))

		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		// One per rendered ConfigMap, made by the namespace pass.
		Expect(configMapGets).To(Equal(3))
		Expect(namespaceGets).To(BeZero())
	})

	It("removes the resources of a namespace whose labels stop matching on its namespace event", func() {
		r, _ := newCountingReconciler()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "relabelled-0", Labels: map[string]string{"team": "a"}}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		mr, req := createManagedResource("relabelled", 0, configMapLiteral("relabelled", "ok"))
		mr.Spec.NamespaceSelector.LabelSelector.MatchLabels = map[string]string{"team": "a"}
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(1))

		By("removing the label and handling only the namespace event")
		ns.Labels = nil
		Expect(k8sClient.Update(ctx, ns)).To(Succeed())
		_, err = newNamespaceController(r).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: ns.Name}})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(BeEmpty())
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "relabelled"}, &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("removes the cluster-scoped resources of a terminating namespace on its namespace event", func() {
		r, _ := newCountingReconciler()
		literal := `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubensync-{{ .Namespace.Name }}
`
		mr, req := createManagedResource("terminating", 1, literal)
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		role := &unstructured.Unstructured{}
		role.SetAPIVersion("rbac.authorization.k8s.io/v1")
		role.SetKind("ClusterRole")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "kubensync-terminating-0"}, role)).To(Succeed())

		By("deleting the namespace, which envtest leaves terminating")
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "terminating-0"}})).To(Succeed())
		_, err = newNamespaceController(r).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "terminating-0"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(BeEmpty())
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "kubensync-terminating-0"}, role)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("removes the resources of a namespace that is already gone on its namespace event", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("gone", 1, configMapLiteral("gone", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		orphan := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "gone-0"}}
		Expect(k8sClient.Create(ctx, orphan)).To(Succeed())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		rendered := mr.Status.CreatedResources
		mr.Status.CreatedResources = append(mr.Status.CreatedResources, automationv1alpha1.CreatedResource{
			ApiVersion: "v1", Kind: "ConfigMap", Name: "orphan", Namespace: "gone-0", TriggerNamespace: "gone-missing",
		})
		Expect(k8sClient.Status().Update(ctx, mr)).To(Succeed())

		_, err = newNamespaceController(r).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "gone-missing"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(Equal(rendered))
		err = k8sClient.Get(ctx, client.ObjectKeyFromObject(orphan), &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("keeps tracking a resource whose delete fails and still deletes the others", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("best-effort", 3, configMapLiteral("best-effort", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		By("narrowing the selector while deletes in best-effort-1 are forbidden")
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		mr.Spec.NamespaceSelector.Regex = "^best-effort-0$"
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		setConfig(r, configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete || !strings.HasPrefix(req.URL.Path, "/api/v1/namespaces/best-effort-1/configmaps/") {
				return next.RoundTrip(req)
			}
			body := `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403,"message":"injected"}`
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}))
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(2))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "best-effort-1", Name: "best-effort"}, &corev1.ConfigMap{})).To(Succeed())
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "best-effort-2", Name: "best-effort"}, &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		By("retrying the delete on the next reconcile")
		setConfig(r, cfg)
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(1))
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "best-effort-1", Name: "best-effort"}, &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stops tracking resources whose kind is no longer served", func() {
		r, _ := newCountingReconciler()
		mr, req := createManagedResource("kind-gone", 1, configMapLiteral("kind-gone", "ok"))
		_, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		rendered := mr.Status.CreatedResources
		mr.Status.CreatedResources = append(mr.Status.CreatedResources, automationv1alpha1.CreatedResource{
			ApiVersion: "gone.kubensync.com/v1", Kind: "Widget", Name: "w", Namespace: "kind-gone-0", TriggerNamespace: "kind-gone-missing",
		})
		Expect(k8sClient.Status().Update(ctx, mr)).To(Succeed())

		_, err = newNamespaceController(r).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "kind-gone-missing"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(Equal(rendered))
	})

	It("delivers changes of a watched object until nothing watches it", func() {
		watchCtx, stop := context.WithCancel(ctx)
		DeferCleanup(stop)
		m := watch.NewManager(watch.MetadataInformers(metadata.NewForConfigOrDie(cfg)), 10*time.Second)
		go func() { _ = m.Start(watchCtx) }()
		watched := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "informer-watched", Namespace: "default"}, Data: map[string]string{"v": "1"}}
		other := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "informer-other", Namespace: "default"}, Data: map[string]string{"v": "1"}}
		Expect(k8sClient.Create(ctx, watched)).To(Succeed())
		Expect(k8sClient.Create(ctx, other)).To(Succeed())
		key := watch.Key{GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Namespace: "default", Name: watched.Name}

		Expect(m.Sync(ctx, "watcher", []watch.Key{key})).To(Succeed())
		Consistently(m.Events()).WithTimeout(time.Second).ShouldNot(Receive())

		By("changing another object of the same namespace")
		other.Data["v"] = "2"
		Expect(k8sClient.Update(ctx, other)).To(Succeed())
		Consistently(m.Events()).WithTimeout(time.Second).ShouldNot(Receive())

		By("changing the watched object")
		watched.Data["v"] = "2"
		Expect(k8sClient.Update(ctx, watched)).To(Succeed())
		Eventually(m.Events()).WithTimeout(10 * time.Second).Should(Receive(Equal(event.TypedGenericEvent[string]{Object: "watcher"})))

		By("changing it after the last watcher stopped watching")
		Expect(m.Sync(ctx, "watcher", nil)).To(Succeed())
		watched.Data["v"] = "3"
		Expect(k8sClient.Update(ctx, watched)).To(Succeed())
		Consistently(m.Events()).WithTimeout(2 * time.Second).ShouldNot(Receive())
	})

	It("reconciles when watched data changes, but not when unwatched data does", func() {
		runManagedResourceController()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "data-watch-0"}})).To(Succeed())
		for _, name := range []string{"watched-src", "unwatched-src"} {
			src := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Data: map[string]string{"v": "1"}}
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
		}
		mr := &automationv1alpha1.ManagedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "data-watch"},
			Spec: automationv1alpha1.ManagedResourceSpec{
				NamespaceSelector: automationv1alpha1.ManagedResourceSpecNamespaceSelector{Regex: "^data-watch-0$"},
				Template: automationv1alpha1.ManagedResourceSpecTemplate{
					Literal: `apiVersion: v1
kind: ConfigMap
metadata:
  name: data-watch
  namespace: {{ .Namespace.Name }}
data:
  watched: {{ index .Data.w "v" | quote }}
  unwatched: {{ index .Data.u "v" | quote }}
`,
					Data: []automationv1alpha1.ManagedResourceSpecTemplateData{
						{Name: "w", Type: automationv1alpha1.ConfigMap, Ref: automationv1alpha1.ManagedResourceSpecTemplateDataRef{Name: "watched-src", Namespace: "default"}, Watch: true},
						{Name: "u", Type: automationv1alpha1.ConfigMap, Ref: automationv1alpha1.ManagedResourceSpecTemplateDataRef{Name: "unwatched-src", Namespace: "default"}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, mr)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, mr))).To(Succeed()) })
		Eventually(configMapValue("data-watch-0", "data-watch", "watched")).WithTimeout(30 * time.Second).Should(Equal("1"))

		By("changing the unwatched data")
		src := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "unwatched-src"}, src)).To(Succeed())
		src.Data["v"] = "2"
		Expect(k8sClient.Update(ctx, src)).To(Succeed())
		Consistently(configMapValue("data-watch-0", "data-watch", "unwatched")).WithTimeout(5 * time.Second).Should(Equal("1"))

		By("changing the watched data")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "watched-src"}, src)).To(Succeed())
		src.Data["v"] = "2"
		Expect(k8sClient.Update(ctx, src)).To(Succeed())
		Eventually(configMapValue("data-watch-0", "data-watch", "watched")).WithTimeout(30 * time.Second).Should(Equal("2"))
		Expect(configMapValue("data-watch-0", "data-watch", "unwatched")()).To(Equal("2"))
	})

	It("reconciles when a watched resource that was missing is created", func() {
		runManagedResourceController()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "data-late-0"}})).To(Succeed())
		mr := &automationv1alpha1.ManagedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "data-late"},
			Spec: automationv1alpha1.ManagedResourceSpec{
				NamespaceSelector: automationv1alpha1.ManagedResourceSpecNamespaceSelector{Regex: "^data-late-0$"},
				Template: automationv1alpha1.ManagedResourceSpecTemplate{
					Literal: configMapLiteral("data-late", `{{ if .Data.sa }}present{{ else }}absent{{ end }}`),
					Data: []automationv1alpha1.ManagedResourceSpecTemplateData{{
						Name: "sa",
						Type: automationv1alpha1.KubernetesResource,
						Ref: automationv1alpha1.ManagedResourceSpecTemplateDataRef{
							ApiVersion: "v1", Kind: "ServiceAccount", Name: "late-sa", Namespace: "data-late-0",
						},
						Watch: true,
					}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, mr)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, mr))).To(Succeed()) })
		Eventually(configMapValue("data-late-0", "data-late", "value")).WithTimeout(30 * time.Second).Should(Equal("absent"))

		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "late-sa", Namespace: "data-late-0"}}
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())
		Eventually(configMapValue("data-late-0", "data-late", "value")).WithTimeout(30 * time.Second).Should(Equal("present"))
	})

	It("reconciles again when another writer marks it not ready", func() {
		runManagedResourceController()
		mr, req := createManagedResource("ready-recovery", 1, configMapLiteral("ready-recovery", "ok"))
		isReady := func() bool {
			Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
			return apimeta.IsStatusConditionTrue(mr.Status.Conditions, automationv1alpha1.ConditionReady)
		}
		Eventually(isReady).WithTimeout(30 * time.Second).Should(BeTrue())

		By("writing a failure the way the namespace controller does")
		apimeta.SetStatusCondition(&mr.Status.Conditions, metav1.Condition{
			Type: automationv1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: "ReconcileError", Message: "transient", ObservedGeneration: mr.Generation,
		})
		Expect(k8sClient.Status().Update(ctx, mr)).To(Succeed())
		Eventually(isReady).WithTimeout(30 * time.Second).Should(BeTrue())
	})

	It("reconciles on spec, metadata and resync events, but not on status writes", func() {
		old := &automationv1alpha1.ManagedResource{ObjectMeta: metav1.ObjectMeta{Name: "events", Generation: 1, ResourceVersion: "1"}}
		updated := func(change func(mr *automationv1alpha1.ManagedResource)) *automationv1alpha1.ManagedResource {
			mr := old.DeepCopy()
			mr.ResourceVersion = "2"
			change(mr)
			return mr
		}
		statusOnly := updated(func(mr *automationv1alpha1.ManagedResource) {
			mr.Status.CreatedResources = []automationv1alpha1.CreatedResource{{Name: "cm"}}
		})
		specChange := updated(func(mr *automationv1alpha1.ManagedResource) { mr.Generation = 2 })
		annotationChange := updated(func(mr *automationv1alpha1.ManagedResource) {
			mr.Annotations = map[string]string{"resync": "now"}
		})
		labelChange := updated(func(mr *automationv1alpha1.ManagedResource) {
			mr.Labels = map[string]string{"kubensync.com/trigger": "true"}
		})

		readyFalse := updated(func(mr *automationv1alpha1.ManagedResource) {
			mr.Status.Conditions = []metav1.Condition{{Type: automationv1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: "ReconcileError"}}
		})
		readyTrue := old.DeepCopy()
		readyTrue.Status.Conditions = []metav1.Condition{{Type: automationv1alpha1.ConditionReady, Status: metav1.ConditionTrue, Reason: "Synced"}}
		stillFalse := readyFalse.DeepCopy()
		stillFalse.ResourceVersion = "3"
		stillFalse.Status.Conditions[0].Message = "another error"

		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: statusOnly})).To(BeFalse())
		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: readyTrue, ObjectNew: readyFalse})).To(BeTrue())
		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: readyFalse})).To(BeTrue())
		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: readyFalse, ObjectNew: stillFalse})).To(BeFalse())
		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: specChange})).To(BeTrue())
		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: annotationChange})).To(BeTrue())
		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: labelChange})).To(BeTrue())
		Expect(managedResourceEvents.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()})).To(BeTrue())
		Expect(managedResourceEvents.Create(event.CreateEvent{Object: old})).To(BeTrue())
		Expect(managedResourceEvents.Delete(event.DeleteEvent{Object: old})).To(BeTrue())
	})
})
