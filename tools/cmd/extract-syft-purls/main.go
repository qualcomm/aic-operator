// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// extract_syft_purls reads a Syft JSON SBOM from standard input and writes its
// non-empty package PURLs to standard output, one per line in sorted order.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type syftSBOM struct {
	Artifacts []struct {
		PURL string `json:"purl"`
	} `json:"artifacts"`
}

func main() {
	var sbom syftSBOM
	if err := json.NewDecoder(os.Stdin).Decode(&sbom); err != nil {
		fmt.Fprintf(os.Stderr, "read Syft JSON: %v\n", err)
		os.Exit(1)
	}

	purls := make(map[string]struct{}, len(sbom.Artifacts))
	for _, artifact := range sbom.Artifacts {
		if artifact.PURL != "" {
			purls[artifact.PURL] = struct{}{}
		}
	}

	sortedPURLs := make([]string, 0, len(purls))
	for purl := range purls {
		sortedPURLs = append(sortedPURLs, purl)
	}
	sort.Strings(sortedPURLs)

	for _, purl := range sortedPURLs {
		fmt.Println(purl)
	}
}
