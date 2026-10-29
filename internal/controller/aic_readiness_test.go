package controller

import (
	"reflect"
	"testing"

	nfr "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	aicv1 "github.com/qualcomm/aic-operator/api/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEvaluateReadinessTransitionsFromMissingToReady(t *testing.T) {
	aic := testAIC()
	missing := evaluateReadiness(aic, readinessObservation{})
	if missing.Conditions[0].Status != metav1.ConditionFalse || missing.Conditions[1].Reason != reasonMissingPrerequisite {
		t.Fatalf("missing prerequisites produced unexpected status: %#v", missing)
	}

	ready := evaluateReadiness(aic, readyObservation())
	if ready.State != aicv1.Operational || ready.Readiness.Ready != 1 || ready.Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("ready observation produced unexpected status: %#v", ready)
	}
}

func TestEvaluateReadinessDetectsSelectorAndVersionDrift(t *testing.T) {
	aic := testAIC()
	observation := readyObservation()
	observation.module.Spec.Selector = map[string]string{"other": "value"}
	observation.module.Spec.DevicePlugin.Container.Image = "registry.invalid/device-plugin:old"
	status := evaluateReadiness(aic, observation)
	if status.State == aicv1.Operational || status.Conditions[0].Status != metav1.ConditionFalse {
		t.Fatalf("drift was not detected: %#v", status)
	}
	if status.NextAction == "" {
		t.Fatal("drift status did not provide a next action")
	}
}

func TestEvaluateReadinessCapsNodeDetails(t *testing.T) {
	aic := testAIC()
	observation := readyObservation()
	for i := 0; i < maxNodeDetails+10; i++ {
		node := readyNode("node-" + string(rune('a'+i%26)))
		node.Name = "node-" + string(rune(i+'a'))
		observation.nodes = append(observation.nodes, node)
	}
	status := evaluateReadiness(aic, observation)
	if len(status.Nodes) != maxNodeDetails {
		t.Fatalf("expected node details to be capped at %d, got %d", maxNodeDetails, len(status.Nodes))
	}
}

func TestReadinessStatusIsStableAcrossRepeatedObservation(t *testing.T) {
	aic := testAIC()
	first := evaluateReadiness(aic, readyObservation())
	first = preserveConditionTransitionTimes(aic.Status, first)
	second := evaluateReadiness(aic, readyObservation())
	second = preserveConditionTransitionTimes(first, second)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated observation changed status: first=%#v second=%#v", first, second)
	}
}

func testAIC() *aicv1.AIC {
	return &aicv1.AIC{ObjectMeta: metav1.ObjectMeta{Name: "aic", Namespace: "default", Generation: 1}, Spec: aicv1.AICSpec{
		SourceImage: "registry.invalid/source:1.0", DriversImage: "registry.invalid/drivers:1.0", DriversVersion: "1.0",
		DevicePluginImage: "registry.invalid/device-plugin", DevPluginVersion: "1.0",
		DeviceAgentImage: "registry.invalid/device-agent", DeviceAgentVersion: "1.0",
		ImageRepoSecret: &v1.LocalObjectReference{Name: "pull-secret"},
	}}
}

func readyObservation() readinessObservation {
	node := readyNode("node-a")
	module := &kmmv1beta1.Module{ObjectMeta: metav1.ObjectMeta{Name: "aic", Namespace: "default"}, Spec: kmmv1beta1.ModuleSpec{
		Selector:     map[string]string{qaicNodeLabel: "true"},
		ModuleLoader: kmmv1beta1.ModuleLoaderSpec{Container: kmmv1beta1.ModuleLoaderContainerSpec{KernelMappings: []kmmv1beta1.KernelMapping{{ContainerImage: "registry.invalid/drivers:1.0", Build: &kmmv1beta1.Build{BuildArgs: []kmmv1beta1.BuildArg{{Name: "QAIC_SRC_IMG", Value: "registry.invalid/source:1.0"}, {Name: "QAIC_VER", Value: "1.0"}}}}}}},
		DevicePlugin: &kmmv1beta1.DevicePluginSpec{Container: kmmv1beta1.DevicePluginContainerSpec{Image: "registry.invalid/device-plugin:1.0"}},
	}, Status: kmmv1beta1.ModuleStatus{ModuleLoader: kmmv1beta1.DaemonSetStatus{AvailableNumber: 1}, DevicePlugin: kmmv1beta1.DaemonSetStatus{AvailableNumber: 1}}}
	return readinessObservation{
		nodes: []v1.Node{node}, module: module, nfr: &nfr.NodeFeatureRule{ObjectMeta: metav1.ObjectMeta{Name: "qcom-aic-nfr", Namespace: "default"}},
		buildCM:     &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "dockerfile-aic", Namespace: "default"}},
		secret:      &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "pull-secret", Namespace: "default"}},
		deviceAgent: &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "aic-qaic-device-agent-ds", Namespace: "default"}, Spec: appsv1.DaemonSetSpec{Template: v1.PodTemplateSpec{Spec: v1.PodSpec{Containers: []v1.Container{{Image: "registry.invalid/device-agent:1.0"}}}}}},
	}
}

func readyNode(name string) v1.Node {
	return v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
		qaicNodeLabel: "true", "kmm.node.kubernetes.io/default.aic.ready": "", "kmm.node.kubernetes.io/default.aic.device-plugin-ready": "",
	}}, Status: v1.NodeStatus{Allocatable: v1.ResourceList{v1.ResourceName(qaicResourceName): resource.MustParse("1")}}}
}
