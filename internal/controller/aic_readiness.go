package controller

import (
	"reflect"
	"sort"
	"strings"

	nfr "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	aicv1 "github.com/qualcomm/aic-operator/api/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	conditionReady              = "Ready"
	conditionDegraded           = "Degraded"
	conditionReconciling        = "Reconciling"
	conditionPrerequisitesReady = "PrerequisitesReady"

	reasonReady                   = "FleetReady"
	reasonNoMatchingNodes         = "NoMatchingNodes"
	reasonMissingPrerequisite     = "MissingPrerequisite"
	reasonSelectorDrift           = "SelectorDrift"
	reasonVersionDrift            = "VersionDrift"
	reasonNodeHardwareMismatch    = "HardwareLabelMissing"
	reasonNodeModuleUnavailable   = "ModuleUnavailable"
	reasonDeviceResourceMissing   = "DeviceResourceMissing"
	reasonDevicePluginUnavailable = "DevicePluginUnavailable"
	reasonAgentUnavailable        = "DeviceAgentUnavailable"

	qaicResourceName = "qualcomm.com/qaic"
	qaicNodeLabel    = "qualcomm.com/qaic"
	maxNodeDetails   = 50
)

type readinessObservation struct {
	nodes         []v1.Node
	module        *kmmv1beta1.Module
	nfr           *nfr.NodeFeatureRule
	buildCM       *v1.ConfigMap
	buildInTreeCM *v1.ConfigMap
	secret        *v1.Secret
	deviceAgent   *appsv1.DaemonSet
}

type readinessIssue struct {
	reason      string
	message     string
	reconciling bool
}

func evaluateReadiness(aic *aicv1.AIC, observation readinessObservation) aicv1.AICStatus {
	status := aicv1.AICStatus{
		State:              aicv1.NotOperational,
		ObservedGeneration: aic.Generation,
	}

	selector := effectiveSelector(aic)
	selected := make([]v1.Node, 0)
	for i := range observation.nodes {
		if labels.SelectorFromSet(selector).Matches(labels.Set(observation.nodes[i].Labels)) {
			selected = append(selected, observation.nodes[i])
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	status.Readiness.Selected = int32(len(selected))

	issues := prerequisiteIssues(aic, observation, selector, int32(len(selected)))
	for _, node := range selected {
		nodeStatus := nodeReadiness(node, observation.module)
		status.Nodes = append(status.Nodes, nodeStatus)
		switch nodeStatus.State {
		case aicv1.AICNodeReady:
			status.Readiness.Ready++
		case aicv1.AICNodeUnavailable:
			status.Readiness.Unavailable++
		default:
			status.Readiness.Degraded++
		}
	}
	if len(issues) == 0 && status.Readiness.Ready != status.Readiness.Selected {
		for _, node := range status.Nodes {
			if node.State != aicv1.AICNodeReady {
				issues = append(issues, readinessIssue{reason: node.Reason, message: nodeAction(node.Reason), reconciling: node.Reason == reasonNodeModuleUnavailable || node.Reason == reasonDevicePluginUnavailable})
				break
			}
		}
	}
	if len(status.Nodes) > maxNodeDetails {
		status.Nodes = status.Nodes[:maxNodeDetails]
	}

	if len(selected) == 0 {
		issues = append(issues, readinessIssue{reason: reasonNoMatchingNodes, message: "no nodes match the effective AIC selector"})
	}
	if len(issues) == 0 && status.Readiness.Ready == status.Readiness.Selected {
		status.State = aicv1.Operational
		status.NextAction = "No action required"
	} else {
		status.NextAction = issues[0].message
	}

	ready := len(issues) == 0 && status.Readiness.Selected > 0 && status.Readiness.Ready == status.Readiness.Selected
	readyReason := reasonReady
	readyMessage := "all selected nodes expose the expected AIC device resource"
	if !ready {
		readyReason = issuesReason(issues, reasonMissingPrerequisite)
		readyMessage = status.NextAction
	}
	status.Conditions = []metav1.Condition{
		newCondition(conditionReady, ready, readyReason, readyMessage),
		newCondition(conditionDegraded, !ready, issuesReason(issues, reasonNoMatchingNodes), status.NextAction),
		newCondition(conditionReconciling, hasReconcilingIssue(issues), reconcilingReason(issues), reconcilingMessage(issues)),
		newCondition(conditionPrerequisitesReady, len(issues) == 0, issuesReason(issues, reasonMissingPrerequisite), prerequisiteMessage(issues)),
	}
	return status
}

func effectiveSelector(aic *aicv1.AIC) map[string]string {
	if len(aic.Spec.Selector) != 0 {
		return aic.Spec.Selector
	}
	return map[string]string{qaicNodeLabel: "true"}
}

func prerequisiteIssues(aic *aicv1.AIC, observation readinessObservation, selector map[string]string, selectedCount int32) []readinessIssue {
	issues := make([]readinessIssue, 0)
	if observation.nfr == nil {
		issues = append(issues, readinessIssue{reason: reasonMissingPrerequisite, message: "check that the owned NodeFeatureRule exists", reconciling: true})
	}
	if observation.module == nil {
		issues = append(issues, readinessIssue{reason: reasonMissingPrerequisite, message: "check that the owned KMM Module exists", reconciling: true})
	} else {
		if !reflect.DeepEqual(observation.module.Spec.Selector, map[string]string{qaicNodeLabel: "true"}) {
			issues = append(issues, readinessIssue{reason: reasonSelectorDrift, message: "restore the owned KMM Module selector to match the AIC node selector", reconciling: false})
		}
		if len(observation.module.Spec.ModuleLoader.Container.KernelMappings) == 0 {
			issues = append(issues, readinessIssue{reason: reasonVersionDrift, message: "check the KMM Module kernel mapping and configured driver versions", reconciling: false})
		} else {
			mapping := observation.module.Spec.ModuleLoader.Container.KernelMappings[0]
			expectedDriversImage := aic.Spec.DriversImage
			if aic.Spec.UseInTreeModules {
				expectedDriversImage += "-inTree"
			}
			if mapping.ContainerImage != expectedDriversImage || mapping.Build == nil || !buildArgMatches(mapping.Build.BuildArgs, "QAIC_SRC_IMG", aic.Spec.SourceImage) || !buildArgMatches(mapping.Build.BuildArgs, "QAIC_VER", aic.Spec.DriversVersion) {
				issues = append(issues, readinessIssue{reason: reasonVersionDrift, message: "align sourceImage, driversImage, and driversVersion with the owned KMM Module", reconciling: false})
			}
		}
		expectedPluginImage := aic.Spec.DevicePluginImage + ":" + aic.Spec.DevPluginVersion
		if observation.module.Spec.DevicePlugin == nil || observation.module.Spec.DevicePlugin.Container.Image != expectedPluginImage {
			issues = append(issues, readinessIssue{reason: reasonVersionDrift, message: "align devicePluginImage and devPluginVersion with the owned KMM Module", reconciling: false})
		}
		if observation.module.Status.DevicePlugin.AvailableNumber < selectedCount {
			issues = append(issues, readinessIssue{reason: reasonDevicePluginUnavailable, message: "check KMM device-plugin pods and their logs", reconciling: true})
		}
		if observation.module.Status.ModuleLoader.AvailableNumber < selectedCount {
			issues = append(issues, readinessIssue{reason: reasonNodeModuleUnavailable, message: "check KMM module-loader pods, kernel support, and firmware", reconciling: true})
		}
	}
	if observation.buildCM == nil || (aic.Spec.UseInTreeModules && observation.buildInTreeCM == nil) {
		issues = append(issues, readinessIssue{reason: reasonMissingPrerequisite, message: "check that the owned KMM build ConfigMap exists", reconciling: true})
	}
	if aic.Spec.ImageRepoSecret != nil && observation.secret == nil {
		issues = append(issues, readinessIssue{reason: reasonMissingPrerequisite, message: "create the referenced image pull Secret in the AIC namespace", reconciling: false})
	}
	if observation.deviceAgent == nil {
		issues = append(issues, readinessIssue{reason: reasonMissingPrerequisite, message: "check that the owned device-agent DaemonSet exists", reconciling: true})
	} else if len(observation.deviceAgent.Spec.Template.Spec.Containers) == 0 || observation.deviceAgent.Spec.Template.Spec.Containers[0].Image != aic.Spec.DeviceAgentImage+":"+aic.Spec.DeviceAgentVersion {
		issues = append(issues, readinessIssue{reason: reasonVersionDrift, message: "align deviceAgentImage and deviceAgentVersion with the owned DaemonSet", reconciling: false})
	}
	if len(aic.Spec.Selector) != 0 && !reflect.DeepEqual(selector, map[string]string{qaicNodeLabel: "true"}) {
		issues = append(issues, readinessIssue{reason: reasonSelectorDrift, message: "the configured selector differs from the selector currently used by the operator", reconciling: false})
	}
	return issues
}

func nodeReadiness(node v1.Node, module *kmmv1beta1.Module) aicv1.AICNodeReadiness {
	result := aicv1.AICNodeReadiness{Name: node.Name, State: aicv1.AICNodeUnavailable, Reason: reasonDeviceResourceMissing}
	if node.Labels[qaicNodeLabel] != "true" {
		result.State = aicv1.AICNodeMismatched
		result.Reason = reasonNodeHardwareMismatch
		return result
	}
	if module != nil {
		moduleReadyLabel := "kmm.node.kubernetes.io/" + module.Namespace + "." + module.Name + ".ready"
		devicePluginReadyLabel := "kmm.node.kubernetes.io/" + module.Namespace + "." + module.Name + ".device-plugin-ready"
		if _, ok := node.Labels[moduleReadyLabel]; !ok {
			result.Reason = reasonNodeModuleUnavailable
			return result
		}
		if _, ok := node.Labels[devicePluginReadyLabel]; !ok {
			result.Reason = reasonDevicePluginUnavailable
			return result
		}
	}
	quantity, exists := node.Status.Allocatable[v1.ResourceName(qaicResourceName)]
	if !exists || quantity.Value() <= 0 {
		return result
	}
	result.State = aicv1.AICNodeReady
	result.Reason = reasonReady
	return result
}

func buildArgMatches(args []kmmv1beta1.BuildArg, name, expected string) bool {
	for _, arg := range args {
		if arg.Name == name {
			return arg.Value == expected || strings.HasSuffix(arg.Value, expected)
		}
	}
	return false
}

func newCondition(conditionType string, status bool, reason, message string) metav1.Condition {
	conditionStatus := metav1.ConditionFalse
	if status {
		conditionStatus = metav1.ConditionTrue
	}
	return metav1.Condition{Type: conditionType, Status: conditionStatus, Reason: reason, Message: message}
}

func issuesReason(issues []readinessIssue, fallback string) string {
	if len(issues) == 0 {
		return reasonReady
	}
	return issues[0].reason
}

func hasReconcilingIssue(issues []readinessIssue) bool {
	for _, issue := range issues {
		if issue.reconciling {
			return true
		}
	}
	return false
}

func reconcilingReason(issues []readinessIssue) string {
	if hasReconcilingIssue(issues) {
		return "OwnedResourcesSettling"
	}
	return "ObservationComplete"
}

func reconcilingMessage(issues []readinessIssue) string {
	if hasReconcilingIssue(issues) {
		return "owned resources or node observations are still settling"
	}
	return "owned resources have been evaluated"
}

func prerequisiteMessage(issues []readinessIssue) string {
	if len(issues) == 0 {
		return "owned prerequisites match the AIC specification"
	}
	return issues[0].message
}

func nodeAction(reason string) string {
	switch reason {
	case reasonNodeHardwareMismatch:
		return "check NFD labels and Qualcomm AI 100 hardware on the affected node"
	case reasonNodeModuleUnavailable:
		return "check KMM module-loader pods, kernel support, and firmware"
	case reasonDevicePluginUnavailable:
		return "check KMM device-plugin pods and their logs"
	case reasonDeviceResourceMissing:
		return "check that the device plugin exposes qualcomm.com/qaic on the affected node"
	default:
		return "inspect the affected node and the owned AIC resources"
	}
}
