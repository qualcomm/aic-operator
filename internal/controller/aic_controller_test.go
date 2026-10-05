/*
Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
SPDX-License-Identifier: BSD-3-Clause-Clear
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aicv1 "github.com/qualcomm/aic-operator/api/v1"
)

type recordingAICReconcilerHelper struct {
	aic   *aicv1.AIC
	calls []string
}

func (h *recordingAICReconcilerHelper) getRequestedAIC(context.Context, types.NamespacedName) (*aicv1.AIC, error) {
	h.calls = append(h.calls, "getRequestedAIC")
	return h.aic, nil
}

func (h *recordingAICReconcilerHelper) finalizeAIC(context.Context, *aicv1.AIC) error {
	h.calls = append(h.calls, "finalizeAIC")
	return nil
}

func (h *recordingAICReconcilerHelper) setFinalizer(context.Context, *aicv1.AIC) error {
	h.calls = append(h.calls, "setFinalizer")
	return nil
}

func (h *recordingAICReconcilerHelper) handleBuildConfigMap(_ context.Context, _ *aicv1.AIC, useInTree bool) error {
	if useInTree {
		h.calls = append(h.calls, "handleInTreeBuildConfigMap")
	} else {
		h.calls = append(h.calls, "handleBuildConfigMap")
	}
	return nil
}

func (h *recordingAICReconcilerHelper) handleKMMModule(context.Context, *aicv1.AIC, aicv1.LoadedModules) error {
	h.calls = append(h.calls, "handleKMMModule")
	return nil
}

func (h *recordingAICReconcilerHelper) handleAICNFDRule(context.Context, *aicv1.AIC) error {
	h.calls = append(h.calls, "handleAICNFDRule")
	return nil
}

func (h *recordingAICReconcilerHelper) handleAICDeviceAgent(context.Context, *aicv1.AIC) error {
	h.calls = append(h.calls, "handleAICDeviceAgent")
	return nil
}

var _ = Describe("AIC Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		It("should successfully reconcile the resource", func() {
			helper := &recordingAICReconcilerHelper{
				aic: &aicv1.AIC{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
				},
			}
			controllerReconciler := &AICReconciler{
				helper: helper,
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(helper.calls).To(Equal([]string{
				"getRequestedAIC",
				"setFinalizer",
				"handleAICNFDRule",
				"handleBuildConfigMap",
				"handleKMMModule",
				"handleAICDeviceAgent",
			}))
		})
	})
})
