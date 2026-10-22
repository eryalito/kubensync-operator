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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
)

var _ = Describe("ManagedResource controller", func() {
	const prefix = "status-write-"

	It("writes status once per reconcile and only when it changes", func() {
		statusUpdates := 0
		base, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		c := interceptor.NewClient(base, interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				statusUpdates++
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		})
		r := &ManagedResourceReconciler{Client: c, apiReader: c, config: cfg}

		for i := range 3 {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s%d", prefix, i)}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		}
		mr := &automationv1alpha1.ManagedResource{
			ObjectMeta: metav1.ObjectMeta{Name: "status-write"},
			Spec: automationv1alpha1.ManagedResourceSpec{
				NamespaceSelector: automationv1alpha1.ManagedResourceSpecNamespaceSelector{Regex: "^" + prefix},
				Template: automationv1alpha1.ManagedResourceSpecTemplate{Literal: `apiVersion: v1
kind: ConfigMap
metadata:
  name: status-write
  namespace: {{ .Namespace.Name }}
`},
			},
		}
		Expect(k8sClient.Create(ctx, mr)).To(Succeed())
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: mr.Name}}

		By("creating the resources in every matching namespace")
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(statusUpdates).To(Equal(1))
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(3))
		Expect(apimeta.IsStatusConditionTrue(mr.Status.Conditions, automationv1alpha1.ConditionReady)).To(BeTrue())

		By("reconciling again without changes")
		statusUpdates = 0
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(statusUpdates).To(BeZero())

		By("removing the resources of a namespace that no longer matches")
		mr.Spec.NamespaceSelector.Regex = "^" + prefix + "[01]$"
		Expect(k8sClient.Update(ctx, mr)).To(Succeed())
		statusUpdates = 0
		_, err = r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(statusUpdates).To(Equal(1))
		Expect(k8sClient.Get(ctx, req.NamespacedName, mr)).To(Succeed())
		Expect(mr.Status.CreatedResources).To(HaveLen(2))
		cm := &corev1.ConfigMap{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: prefix + "2", Name: "status-write"}, cm)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})
