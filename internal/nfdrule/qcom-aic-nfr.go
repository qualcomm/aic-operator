/*
Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
SPDX-License-Identifier: BSD-3-Clause-Clear.
*/

package nfdrule

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nfr "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	aicv1 "github.com/qualcomm/aic-operator/api/v1"
)

const (
	qcom_aic_nfrmanifest = "/opt/aic-manifests/qcom-aic-nfr.yaml"
)

type NFDRuleAPI interface {
	SetNFRasDesired(ctx context.Context, nfrObj *nfr.NodeFeatureRule, aic *aicv1.AIC) error
}

type nfdRule struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewNFDRule(client client.Client, scheme *runtime.Scheme) NFDRuleAPI {
	return &nfdRule{
		client: client,
		scheme: scheme,
	}
}

func (nfrstruct *nfdRule) SetNFRasDesired(ctx context.Context, nfrObj *nfr.NodeFeatureRule, aic *aicv1.AIC) error {
	err := parseNFR_Manifest(ctx, nfrObj, aic)
	if err != nil {
		return fmt.Errorf("failed to set NodeFeatureRule: %v", err)
	}
	return controllerutil.SetControllerReference(aic, nfrObj, nfrstruct.scheme)
}

func parseNFR_Manifest(ctx context.Context, nfrObj *nfr.NodeFeatureRule, aic *aicv1.AIC) error {
	raw_manifest, err := os.ReadFile(qcom_aic_nfrmanifest)
	logger := log.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("error reading NFR manifest %s: %w", qcom_aic_nfrmanifest, err)
	}
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
	regex, _ := regexp.Compile(`\b(\w*kind:\w*)\B.*\b`)
	kind := strings.TrimSpace(strings.Split(regex.FindString(string(raw_manifest)), ":")[1])
	logger.Info("Resource identified kind", "kind", kind)
	_, _, err = s.Decode(raw_manifest, nil, nfrObj)
	if err != nil {
		return fmt.Errorf("error decoding NodeFeatureRule resource in manifest %s: %w", qcom_aic_nfrmanifest, err)
	}
	nfrObj.Namespace = aic.Namespace
	return nil
}
