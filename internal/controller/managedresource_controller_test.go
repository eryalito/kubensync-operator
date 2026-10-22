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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
)

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
	return &ManagedResourceReconciler{Client: c, apiReader: c, config: cfg}, &statusWrites
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
	return &ManagedResourceReconciler{Client: c, apiReader: c, config: cfg}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// recordConfigMapNamespaces makes r record the namespaces whose ConfigMaps it touches.
func recordConfigMapNamespaces(r *ManagedResourceReconciler) map[string]bool {
	touched := map[string]bool{}
	r.config = configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/configmaps") {
			touched[strings.Split(strings.TrimPrefix(req.URL.Path, "/api/v1/namespaces/"), "/")[0]] = true
		}
		return next.RoundTrip(req)
	})
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
		Expect(*statusWrites).To(Equal(1))
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
		r.config = configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
			if creates == 3 {
				cancelReconcile()
			}
			resp, err := next.RoundTrip(req)
			if err == nil && req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/configmaps") {
				creates++
			}
			return resp, err
		})
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
		r.config = configWithTransport(func(next http.RoundTripper, req *http.Request) (*http.Response, error) {
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
		})

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
})
