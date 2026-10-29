/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Based on code from https://github.com/yevgeny-shnaidman/amd-gpu-operator

Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
SPDX-License-Identifier: BSD-3-Clause-Clear
Not a contribution.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	nfr "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aicv1 "github.com/qualcomm/aic-operator/api/v1"
	"github.com/qualcomm/aic-operator/internal/deviceagent"
	"github.com/qualcomm/aic-operator/internal/kmmmodule"
	"github.com/qualcomm/aic-operator/internal/nfdrule"
)

// AICReconciler reconciles an AIC object
type AICReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	helper AICReconcilerHelperAPI
}

//+kubebuilder:rbac:groups=aic.qualcomm.com,resources=aics,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aic.qualcomm.com,resources=aics/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aic.qualcomm.com,resources=aics/finalizers,verbs=update
//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=modules,verbs=get;list;watch;create;patch;update;delete
//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=modules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nfd.openshift.io,resources=nodefeaturerules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nfd.openshift.io,resources=nodefeaturerules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces;serviceaccounts;pods;pods/exec;pods/attach;services;services/finalizers;endpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps;secrets;nodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=get;list;watch;create;update;patch;delete

func NewAICReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	kmmHandler kmmmodule.KMMModuleAPI,
	nfdHandler nfdrule.NFDRuleAPI,
	deviceAgentHandler deviceagent.DeviceAgentAPI) *AICReconciler {
	helper := newAICReconcilerHelper(client, kmmHandler, nfdHandler, deviceAgentHandler)
	return &AICReconciler{
		Client: client,
		Scheme: scheme,
		helper: helper,
	}
}

const (
	aicFinalizer = "aic-operator.node.kubernetes.io/finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *AICReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	res := ctrl.Result{}
	logger := log.FromContext(ctx)

	aic, err := r.helper.getRequestedAIC(ctx, req.NamespacedName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("Module deleted")
			return ctrl.Result{}, nil
		}

		return res, fmt.Errorf("failed to get the requested %s KMMO CR: %w", req.NamespacedName, err)
	}

	if aic.GetDeletionTimestamp() != nil {
		err = r.helper.finalizeAIC(ctx, aic)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to finalize AIC %s: %v", req.NamespacedName, err)
		}
		return ctrl.Result{}, nil
	}

	err = r.helper.setFinalizer(ctx, aic)
	if err != nil {
		return res, fmt.Errorf("failed to set finalizer for AIC %s: %v", req.NamespacedName, err)
	}

	logger.Info("start NFR reconciliation")
	err = r.helper.handleAICNFDRule(ctx, aic)
	if err != nil {
		return res, fmt.Errorf("failed to handle NFR creation %s: %v", req.NamespacedName, err)
	}

	logger.Info("start build configmap reconciliation")
	// Always want to have the ConfigMap that can build module images
	err = r.helper.handleBuildConfigMap(ctx, aic, false)
	if err != nil {
		return res, fmt.Errorf("failed to handle build ConfigMap for DeviceConfig %s: %v", req.NamespacedName, err)
	}
	// but if we are enabling inTreeModules, we need one that creates blanks
	if aic.Spec.UseInTreeModules {
		err = r.helper.handleBuildConfigMap(ctx, aic, true)
		if err != nil {
			return res, fmt.Errorf("failed to handle build ConfigMap for DeviceConfig %s: %v", req.NamespacedName, err)
		}
	}

	logger.Info("start KMM reconciliation")
	err = r.helper.handleKMMModule(ctx, aic, aicv1.Qaic_loaded)
	if err != nil {
		return res, fmt.Errorf("failed to handle KMM module for AIC %s: %v", req.NamespacedName, err)
	}
	logger.Info("start Device-Agent DaemonSet reconciliation")
	err = r.helper.handleAICDeviceAgent(ctx, aic)
	if err != nil {
		return res, fmt.Errorf("failed to handle SOC Reset DaemonSet creation %s: %v", req.NamespacedName, err)
	}
	if err = r.updateReadinessStatus(ctx, aic); err != nil {
		return res, fmt.Errorf("failed to update AIC readiness status %s: %w", req.NamespacedName, err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AICReconciler) SetupWithManager(mgr ctrl.Manager, reconciliationInterval time.Duration) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aicv1.AIC{}).
		Owns(&kmmv1beta1.Module{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&v1.ConfigMap{}).
		Owns(&nfr.NodeFeatureRule{}).
		Watches(&v1.Node{}, handler.EnqueueRequestsFromMapFunc(r.mapNodeToAIC)).
		Watches(&v1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.mapSecretToAIC)).
		Complete(r)
}

func (r *AICReconciler) updateReadinessStatus(ctx context.Context, aic *aicv1.AIC) error {
	observation := readinessObservation{}
	nodes := &v1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	observation.nodes = nodes.Items

	module := &kmmv1beta1.Module{}
	if err := r.Get(ctx, types.NamespacedName{Name: aic.Name, Namespace: aic.Namespace}, module); err == nil {
		observation.module = module
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get KMM Module: %w", err)
	}

	nfrObj := &nfr.NodeFeatureRule{}
	if err := r.Get(ctx, types.NamespacedName{Name: "qcom-aic-nfr", Namespace: aic.Namespace}, nfrObj); err == nil {
		observation.nfr = nfrObj
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get NodeFeatureRule: %w", err)
	}

	buildCM := &v1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: getDockerfileCMName(aic, false), Namespace: aic.Namespace}, buildCM); err == nil {
		observation.buildCM = buildCM
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get build ConfigMap: %w", err)
	}
	if aic.Spec.UseInTreeModules {
		inTreeCM := &v1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: getDockerfileCMName(aic, true), Namespace: aic.Namespace}, inTreeCM); err == nil {
			observation.buildInTreeCM = inTreeCM
		} else if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get in-tree build ConfigMap: %w", err)
		}
	}

	if aic.Spec.ImageRepoSecret != nil {
		secret := &v1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: aic.Spec.ImageRepoSecret.Name, Namespace: aic.Namespace}, secret); err == nil {
			observation.secret = secret
		} else if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get image pull Secret: %w", err)
		}
	}

	deviceAgent := &appsv1.DaemonSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: aic.Name + "-qaic-device-agent-ds", Namespace: aic.Namespace}, deviceAgent); err == nil {
		observation.deviceAgent = deviceAgent
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get device-agent DaemonSet: %w", err)
	}

	newStatus := evaluateReadiness(aic, observation)
	newStatus = preserveConditionTransitionTimes(aic.Status, newStatus)
	if reflect.DeepEqual(aic.Status, newStatus) {
		return nil
	}
	oldStatus := aic.Status
	copy := aic.DeepCopy()
	aic.Status = newStatus
	if err := r.Status().Patch(ctx, aic, client.MergeFrom(copy)); err != nil {
		return err
	}
	if oldStatus.State != newStatus.State || oldStatus.Readiness != newStatus.Readiness {
		log.FromContext(ctx).Info("AIC fleet readiness changed", "state", newStatus.State, "selected", newStatus.Readiness.Selected, "ready", newStatus.Readiness.Ready, "degraded", newStatus.Readiness.Degraded, "unavailable", newStatus.Readiness.Unavailable, "nextAction", newStatus.NextAction)
	}
	return nil
}

func preserveConditionTransitionTimes(oldStatus, newStatus aicv1.AICStatus) aicv1.AICStatus {
	for i := range newStatus.Conditions {
		for j := range oldStatus.Conditions {
			if newStatus.Conditions[i].Type == oldStatus.Conditions[j].Type && newStatus.Conditions[i].Status == oldStatus.Conditions[j].Status && newStatus.Conditions[i].Reason == oldStatus.Conditions[j].Reason && newStatus.Conditions[i].Message == oldStatus.Conditions[j].Message {
				newStatus.Conditions[i].LastTransitionTime = oldStatus.Conditions[j].LastTransitionTime
				break
			}
		}
		if newStatus.Conditions[i].LastTransitionTime.IsZero() {
			newStatus.Conditions[i].LastTransitionTime = metav1.Now()
		}
	}
	return newStatus
}

func (r *AICReconciler) mapNodeToAIC(ctx context.Context, obj client.Object) []ctrl.Request {
	node, ok := obj.(*v1.Node)
	if !ok {
		return nil
	}
	list := &aicv1.AICList{}
	if err := r.List(ctx, list); err != nil {
		return nil
	}
	requests := make([]ctrl.Request, 0)
	for _, aic := range list.Items {
		if labels.SelectorFromSet(effectiveSelector(&aic)).Matches(labels.Set(node.Labels)) {
			requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Name: aic.Name, Namespace: aic.Namespace}})
		}
	}
	return requests
}

func (r *AICReconciler) mapSecretToAIC(ctx context.Context, obj client.Object) []ctrl.Request {
	secret, ok := obj.(*v1.Secret)
	if !ok {
		return nil
	}
	list := &aicv1.AICList{}
	if err := r.List(ctx, list, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}
	requests := make([]ctrl.Request, 0)
	for _, aic := range list.Items {
		if aic.Spec.ImageRepoSecret != nil && aic.Spec.ImageRepoSecret.Name == secret.Name {
			requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Name: aic.Name, Namespace: aic.Namespace}})
		}
	}
	return requests
}

type AICReconcilerHelperAPI interface {
	getRequestedAIC(ctx context.Context, namespacedName types.NamespacedName) (*aicv1.AIC, error)
	finalizeAIC(ctx context.Context, aic *aicv1.AIC) error
	setFinalizer(ctx context.Context, aic *aicv1.AIC) error
	handleBuildConfigMap(ctx context.Context, devConfig *aicv1.AIC, useInTree bool) error
	handleKMMModule(ctx context.Context, devConfig *aicv1.AIC, loadedMods aicv1.LoadedModules) error
	handleAICNFDRule(ctx context.Context, aic *aicv1.AIC) error
	handleAICDeviceAgent(ctx context.Context, aic *aicv1.AIC) error
}

type AICReconcilerHelper struct {
	client             client.Client
	kmmHandler         kmmmodule.KMMModuleAPI
	nfdHandler         nfdrule.NFDRuleAPI
	deviceAgentHandler deviceagent.DeviceAgentAPI
}

func newAICReconcilerHelper(client client.Client,
	kmmHandler kmmmodule.KMMModuleAPI, nfdHandler nfdrule.NFDRuleAPI, deviceAgentHandler deviceagent.DeviceAgentAPI) AICReconcilerHelperAPI {
	return &AICReconcilerHelper{
		client:             client,
		kmmHandler:         kmmHandler,
		nfdHandler:         nfdHandler,
		deviceAgentHandler: deviceAgentHandler,
	}
}
func (aicrh *AICReconcilerHelper) getRequestedAIC(ctx context.Context, namespacedName types.NamespacedName) (*aicv1.AIC, error) {
	aic := aicv1.AIC{}

	if err := aicrh.client.Get(ctx, namespacedName, &aic); err != nil {
		return nil, fmt.Errorf("failed to get AIC %s: %w", namespacedName, err)
	}
	return &aic, nil
}

func (aicrh *AICReconcilerHelper) setFinalizer(ctx context.Context, aic *aicv1.AIC) error {
	if controllerutil.ContainsFinalizer(aic, aicFinalizer) {
		return nil
	}

	aicCopy := aic.DeepCopy()
	controllerutil.AddFinalizer(aic, aicFinalizer)
	return aicrh.client.Patch(ctx, aic, client.MergeFrom(aicCopy))
}

func (aicrh *AICReconcilerHelper) finalizeAIC(ctx context.Context, aic *aicv1.AIC) error {
	logger := log.FromContext(ctx)
	var errs []error

	// Delete SocReset DS owned by AIC
	socresetDs := appsv1.DaemonSet{}
	nsName := types.NamespacedName{
		Namespace: aic.Namespace,
		Name:      aic.Name + "-qaic-socreset-ds",
	}

	// Try to delete the SocReset DaemonSet
	err := aicrh.client.Get(ctx, nsName, &socresetDs)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to get the requested qaic-socreset-ds daemonset %s: %v", nsName, err))
		}
	} else {
		logger.Info("deleting qaic-socreset daemonset", "daemonset", nsName)
		if err = aicrh.client.Delete(ctx, &socresetDs); client.IgnoreNotFound(err) != nil {
			errs = append(errs, fmt.Errorf("failed to delete qaic-socreset-ds daemonset: %w", err))
		}
	}

	// Delete KMM.Module owned by AIC
	mod := kmmv1beta1.Module{}
	nsName = types.NamespacedName{
		Namespace: aic.Namespace,
		Name:      aic.Name,
	}
	err = aicrh.client.Get(ctx, nsName, &mod)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to get the requested Module %s: %w", nsName, err))
		}
	} else {
		logger.Info("deleting KMM Module", "module", nsName)
		if err = aicrh.client.Delete(ctx, &mod); client.IgnoreNotFound(err) != nil {
			errs = append(errs, fmt.Errorf("failed to delete KMM Module: %w", err))
		}
	}

	// Delete NFR owned by AIC
	nfrObj := nfr.NodeFeatureRule{}
	nsName = types.NamespacedName{
		Namespace: aic.Namespace,
		Name:      "qcom-aic-nfr",
	}
	err = aicrh.client.Get(ctx, nsName, &nfrObj)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to get the requested NFR %s: %w", nsName, err))
		}
	} else {
		logger.Info("deleting NFR CR", "NFR", nsName)
		if err = aicrh.client.Delete(ctx, &nfrObj); client.IgnoreNotFound(err) != nil {
			errs = append(errs, fmt.Errorf("failed to delete NFR: %w", err))
		}
	}

	// Join all errors
	if combinedErr := errors.Join(errs...); combinedErr != nil {
		return combinedErr
	}

	// If we got here, all resources are either deleted or not found
	logger.Info("All resources cleaned up, removing finalizer", "AIC", aic.Name)
	aicCopy := aic.DeepCopy()
	controllerutil.RemoveFinalizer(aic, aicFinalizer)
	return aicrh.client.Patch(ctx, aic, client.MergeFrom(aicCopy))
}

func (aicrh *AICReconcilerHelper) handleBuildConfigMap(ctx context.Context, aic *aicv1.AIC, useInTree bool) error {
	buildDockerfileCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: aic.Namespace,
			Name:      getDockerfileCMName(aic, useInTree),
		},
	}
	logger := log.FromContext(ctx)
	opRes, err := controllerutil.CreateOrPatch(ctx, aicrh.client, buildDockerfileCM, func() error {
		return aicrh.kmmHandler.SetBuildConfigMapAsDesired(buildDockerfileCM, aic, useInTree)
	})

	if err == nil {
		logger.Info("Reconciled KMM build dockerfile ConfigMap", "name", buildDockerfileCM.Name, "result", opRes)
	}

	return err
}

func (aicrh *AICReconcilerHelper) handleKMMModule(ctx context.Context, aic *aicv1.AIC, loadedMods aicv1.LoadedModules) error {
	if loadedMods > aicv1.None_loaded {
		return fmt.Errorf("invalid loadedModule type = %d", loadedMods)
	}

	kmmMod := &kmmv1beta1.Module{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: aic.Namespace,
			Name:      aic.Name,
		},
	}
	logger := log.FromContext(ctx)
	opRes, err := controllerutil.CreateOrPatch(ctx, aicrh.client, kmmMod, func() error {
		return aicrh.kmmHandler.SetKMMModuleAsDesired(kmmMod, aic, loadedMods)
	})

	if err == nil {
		logger.Info("Reconciled KMM Module", "name", kmmMod.Name, "result", opRes)
	}

	return err
}

func (aicrh *AICReconcilerHelper) handleAICNFDRule(ctx context.Context, aic *aicv1.AIC) error {

	nfrObj := &nfr.NodeFeatureRule{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: aic.Namespace,
			Name:      "qcom-aic-nfr",
		},
	}
	logger := log.FromContext(ctx)
	opRes, err := controllerutil.CreateOrPatch(ctx, aicrh.client, nfrObj, func() error {
		return aicrh.nfdHandler.SetNFRasDesired(ctx, nfrObj, aic)
	})
	if err != nil {
		return err
	}

	logger.Info("Reconciled NFR", "name", nfrObj.Name, "result", opRes)
	return nil

}

func (aicrh *AICReconcilerHelper) handleAICDeviceAgent(ctx context.Context, aic *aicv1.AIC) error {
	deviceagentdsObj := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: aic.Namespace,
			Name:      aic.Name + "-qaic-device-agent-ds",
		},
	}
	logger := log.FromContext(ctx)
	opRes, err := controllerutil.CreateOrPatch(ctx, aicrh.client, deviceagentdsObj, func() error {
		return aicrh.deviceAgentHandler.SetDeviceAgentDSasDesired(deviceagentdsObj, aic)
	})
	if err != nil {
		return err
	}

	logger.Info("Reconciled Device Agent DaemonSet", "name", deviceagentdsObj.Name, "result", opRes)
	return nil
}
func getDockerfileCMName(aic *aicv1.AIC, useInTree bool) string {
	if useInTree {
		return "dockerfile-intree-" + aic.Name
	} else {
		return "dockerfile-" + aic.Name
	}
}
