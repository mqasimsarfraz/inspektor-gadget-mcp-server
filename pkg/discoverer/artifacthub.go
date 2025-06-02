// Copyright 2025 The Inspektor Gadget authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package discoverer

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const SourceArtifactHub = "artifacthub"

type ArtifacthubPackages struct {
	Packages []ArtifacthubPackage `json:"packages"`
}

type ArtifacthubPackage struct {
	Name           string `json:"name"`
	NormalizedName string `json:"normalized_name"`
	Description    string `json:"description"`
	Official       bool   `json:"official"`
	CNCF           bool   `json:"cncf"`
	Deprecated     bool   `json:"deprecated"`
	Version        string `json:"version"`
}

type ArtifacthubPackageDetails struct {
	ContainersImages []struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	} `json:"containers_images"`
}

type artifactHubDiscoverer struct{}

func NewArtifactHubDiscoverer() Discoverer {
	return &artifactHubDiscoverer{}
}

func (d *artifactHubDiscoverer) ListImages() ([]string, error) {
	packages, err := d.listPackages()
	if err != nil {
		return nil, fmt.Errorf("listing packages from Artifact Hub: %w", err)
	}

	var images []string
	for _, pkg := range packages.Packages {
		if !pkg.Official || !pkg.CNCF || pkg.Deprecated {
			log.Debug("Skipping gadget", "normalized_name", pkg.NormalizedName, "official", pkg.Official, "cncf", pkg.CNCF, "deprecated", pkg.Deprecated)
			continue
		}
		image, err := d.getPackageImage(pkg.NormalizedName)
		if err != nil {
			return nil, fmt.Errorf("getting image for package %s: %w", pkg.NormalizedName, err)
		}
		images = append(images, image)
	}
	return images, nil
}

func (d *artifactHubDiscoverer) listPackages() (*ArtifacthubPackages, error) {
	// Gadget packages are listed under kind 22 in Artifact Hub
	url := "https://artifacthub.io/api/v1/packages/search?kind=22&limit=60"
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching packages from Artifact Hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from Artifact Hub: %d", resp.StatusCode)
	}

	var packages ArtifacthubPackages
	if err = json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return nil, fmt.Errorf("decoding packages from Artifact Hub: %w", err)
	}

	return &packages, nil
}

func (d *artifactHubDiscoverer) getPackageImage(name string) (string, error) {
	url := fmt.Sprintf("https://artifacthub.io/api/v1/packages/inspektor-gadget/gadgets/%s", name)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching package details from Artifact Hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code from Artifact Hub: %d", resp.StatusCode)
	}

	var details ArtifacthubPackageDetails
	if err = json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return "", fmt.Errorf("decoding package details from Artifact Hub: %w", err)
	}
	if len(details.ContainersImages) == 0 {
		return "", fmt.Errorf("no container images found for package %s", name)
	}
	return details.ContainersImages[0].Image, nil
}
