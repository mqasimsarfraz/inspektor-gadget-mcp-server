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

package deployer

import (
	"context"
	"fmt"
)

const (
	KubernetesEnv = "kubernetes"
	LinuxEnv      = "linux"
)

// Deployer defines the interface for managing Inspektor Gadget deployment on a target system.
type Deployer interface {
	// Deploy deploys Inspektor Gadget on the target system
	Deploy(ctx context.Context, opts ...RunOption) error
	// Undeploy removes Inspektor Gadget from the target system
	Undeploy(ctx context.Context, opts ...RunOption) error
	// IsDeployed check if Inspektor Gadget is deployed on the target system by the given deployer
	IsDeployed(ctx context.Context, opts ...RunOption) (bool, error)
}

type RunOption func(*config)

type config struct {
	chartUrl              string
	releaseName           string
	namespace             string
	skipNamespaceCreation bool
}

// NewDeployer creates a new Deployer based on the environment
func NewDeployer(env string) (Deployer, error) {
	switch env {
	case KubernetesEnv:
		return newHelmDeployer()
	}

	return nil, fmt.Errorf("unsupported environment: %s", env)
}

func (c *config) applyOptions(opts ...RunOption) {
	for _, opt := range opts {
		opt(c)
	}
}

func WithChartURL(url string) RunOption {
	return func(c *config) {
		c.chartUrl = url
	}
}

func WithReleaseName(name string) RunOption {
	return func(c *config) {
		c.releaseName = name
	}
}

func WithNamespace(namespace string) RunOption {
	return func(c *config) {
		c.namespace = namespace
	}
}

func WithSkipNamespaceCreation(skip bool) RunOption {
	return func(c *config) {
		c.skipNamespaceCreation = skip
	}
}
