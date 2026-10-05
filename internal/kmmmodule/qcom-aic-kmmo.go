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

package kmmmodule

import (
	_ "embed"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/a8m/envsubst/parse"
	aicv1 "github.com/qualcomm/aic-operator/api/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
)

const (
	aicDriverModuleName = "qaic"
	imageFirmwarePath   = "/firmware"
)

var (
	//go:embed dockerfiles/driversDockerfile.txt
	buildDockerfile string
	//go:embed dockerfiles/inTreeDockerfile.txt
	inTreeDockerfile string
)

//go:generate mockgen -source=kmmmodule.go -package=kmmmodule -destination=mock_kmmmodule.go KMMModuleAPI
type KMMModuleAPI interface {
	SetBuildConfigMapAsDesired(buildCM *v1.ConfigMap, aic *aicv1.AIC, useInTree bool) error
	SetKMMModuleAsDesired(mod *kmmv1beta1.Module, aic *aicv1.AIC, loadedMods aicv1.LoadedModules) error
}

type kmmModule struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewKMMModule(client client.Client, scheme *runtime.Scheme) KMMModuleAPI {
	return &kmmModule{
		client: client,
		scheme: scheme,
	}
}

func (km *kmmModule) SetBuildConfigMapAsDesired(buildCM *v1.ConfigMap, aic *aicv1.AIC, useInTree bool) error {
	if buildCM.Data == nil {
		buildCM.Data = make(map[string]string)
	}

	if useInTree {
		buildCM.Data["dockerfile"] = inTreeDockerfile
	} else {
		buildCM.Data["dockerfile"] = buildDockerfile
	}
	return controllerutil.SetControllerReference(aic, buildCM, km.scheme)
}

func (km *kmmModule) SetKMMModuleAsDesired(mod *kmmv1beta1.Module, aic *aicv1.AIC, loadedMods aicv1.LoadedModules) error {
	err := setKMMModuleLoader(mod, aic, loadedMods)
	if err != nil {
		return fmt.Errorf("failed to set KMM Module: %v", err)
	}
	err = setKMMDevicePlugin(mod, aic)
	if err != nil {
		return fmt.Errorf("failed to set KMM DevicePlugin: %v", err)
	}
	return controllerutil.SetControllerReference(aic, mod, km.scheme)
}

func setKMMModuleLoader(mod *kmmv1beta1.Module, aic *aicv1.AIC, loadedMods aicv1.LoadedModules) error {
	//When UseInTreeModules is defined, its only applicable to kernels with the modules available.
	useInTree := (aic.Spec.UseInTreeModules && loadedMods == aicv1.Qaic_loaded)
	driversVersion := aic.Spec.DriversVersion
	modVars := []string{
		"MOD_NAMESPACE=" + aic.Namespace,
	}
	parser := parse.New("mod_replace", modVars, &parse.Restrictions{})

	if driversVersion == "" {
		return fmt.Errorf("driversVersion in AIC spec is not set, exiting")
	}

	sourceImage := aic.Spec.SourceImage
	if sourceImage == "" {
		return fmt.Errorf("sourceImage in AIC spec is not set, exiting")
	}
	sourceImage, err := parser.Parse(sourceImage)
	if err != nil {
		return fmt.Errorf("failed to replace %q, %w", sourceImage, err)
	}

	driversImage := aic.Spec.DriversImage
	if driversImage == "" {
		return fmt.Errorf("driversImage in AIC spec is not set, exiting")
	}
	if useInTree {
		driversImage = driversImage + "-inTree"
	}

	// ModulesLoadingOrder defines the dependency between kernel modules loading.
	// The list order should be: upmost module, then the module it depends on and so on.
	// Example: if moduleA depends on first loading moduleB, and moduleB depends on first loading moduleC
	// the entry should look:
	// ModulesLoadingOrder:
	//    - moduleA
	//    - moduleB
	//    - moduleC
	// In order to load all 3 modules, moduleA shoud be defined in the ModuleName parameter of this struct
	modulesLoadingOrder := make([]string, 0)
	modulesLoadingOrder = append(modulesLoadingOrder, "qaic", "qrtr-mhi", "switchtec", "mhi")
	mod.Spec.ModuleLoader = &kmmv1beta1.ModuleLoaderSpec{
		Container: kmmv1beta1.ModuleLoaderContainerSpec{
			Modprobe: kmmv1beta1.ModprobeSpec{
				ModuleName:          aicDriverModuleName,
				FirmwarePath:        imageFirmwarePath,
				ModulesLoadingOrder: modulesLoadingOrder,
			},
			KernelMappings: []kmmv1beta1.KernelMapping{
				{
					Regexp:         "^.+$",
					ContainerImage: driversImage,
					Build: &kmmv1beta1.Build{
						DockerfileConfigMap: &v1.LocalObjectReference{
							Name: getDockerfileCMName(aic, useInTree),
						},
						BuildArgs: []kmmv1beta1.BuildArg{
							{
								Name:  "QAIC_SRC_IMG",
								Value: sourceImage,
							},
							{
								Name:  "QAIC_VER",
								Value: driversVersion,
							},
						},
					},
				},
			},
		},
	}

	modulesToRemove := make([]string, 0)
	switch loadedMods {
	case aicv1.Qaic_loaded:
		// Use hyphen (qrtr-mhi) to match the actual .ko filename
		modulesToRemove = append(modulesToRemove, "qaic", "qrtr-mhi", "switchtec", "mhi")
	case aicv1.Mhi_loaded:
		modulesToRemove = append(modulesToRemove, "qrtr-mhi", "mhi")
	}
	if len(modulesToRemove) != 0 || !useInTree {
		mod.Spec.ModuleLoader.Container.InTreeModulesToRemove = modulesToRemove
	}

	mod.Spec.ModuleLoader.ServiceAccountName = "aic-operator-kmm-module-loader"
	mod.Spec.ImageRepoSecret = aic.Spec.ImageRepoSecret
	mod.Spec.Selector = getNodeSelector()
	return nil
}

func setKMMDevicePlugin(mod *kmmv1beta1.Module, aic *aicv1.AIC) error {
	devPluginVersion := aic.Spec.DevPluginVersion
	if devPluginVersion == "" {
		return fmt.Errorf("devPluginVersion not set in AIC Spec")
	}

	modVars := []string{
		"MOD_NAMESPACE=" + aic.Namespace,
	}
	parser := parse.New("mod_replace", modVars, &parse.Restrictions{})
	devicePluginImage := aic.Spec.DevicePluginImage
	if devicePluginImage == "" {
		return fmt.Errorf("devivePluginImage not set in AIC Spec")
	} else {
		devicePluginImage = devicePluginImage + ":" + devPluginVersion
	}
	devicePluginImage, err := parser.Parse(devicePluginImage)
	if err != nil {
		return fmt.Errorf("failed to replace %q, %w", devicePluginImage, err)
	}

	hostPathDirectory := v1.HostPathDirectory
	mod.Spec.DevicePlugin = &kmmv1beta1.DevicePluginSpec{
		ServiceAccountName: "aic-operator-kmm-device-plugin",
		Container: kmmv1beta1.DevicePluginContainerSpec{
			Image: devicePluginImage,
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "udev-run",
					MountPath: "/run/udev",
					ReadOnly:  true,
				},
			},
		},
		Volumes: []v1.Volume{
			{
				Name: "udev-run",
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: "/run/udev",
						Type: &hostPathDirectory,
					},
				},
			},
		},
	}
	return nil
}

func getDockerfileCMName(aic *aicv1.AIC, useInTree bool) string {
	if useInTree {
		return "dockerfile-intree-" + aic.Name
	} else {
		return "dockerfile-" + aic.Name
	}
}

func getNodeSelector() map[string]string {

	selectors := map[string]string{"qualcomm.com/qaic": "true"}
	return selectors
}
