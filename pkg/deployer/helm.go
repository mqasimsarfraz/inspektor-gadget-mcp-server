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
	"log/slog"
	"net/http"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"

	"github.com/inspektor-gadget/inspektor-gadget/cmd/kubectl-gadget/utils"
)

const (
	LabelKeyManagedBy   = "inspektor-gadget.io/managed-by"
	LabelValueManagedBy = "ig-mcp-server"
)

const defaultHttpTimeout = 5 * time.Second

var log = slog.Default().With("component", "inspektor-gadget-helm-deployer")

var (
	ErrChartURLNotSet        = fmt.Errorf("chart URL not set")
	ErrNotDeployedByDeployer = fmt.Errorf("not deployed by deployer")
)

type helmDeployer struct {
	registryClient *registry.Client
}

func newHelmDeployer() (*helmDeployer, error) {
	hc := http.Client{Timeout: defaultHttpTimeout}
	opts := []registry.ClientOption{
		registry.ClientOptHTTPClient(&hc),
	}
	rc, err := registry.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("create registry client: %w", err)
	}

	return &helmDeployer{
		registryClient: rc,
	}, nil
}

func (h *helmDeployer) Deploy(ctx context.Context, opts ...RunOption) error {
	var cfg config
	cfg.applyOptions(opts...)
	chartUrl := cfg.chartUrl
	if chartUrl == "" {
		return ErrChartURLNotSet
	}
	releaseName := cfg.releaseName
	if releaseName == "" {
		releaseName = "gadget"
	}
	namespace := cfg.namespace
	if namespace == "" {
		namespace = "gadget"
	}

	actionCfg, err := h.getActionConfig(namespace)
	if err != nil {
		return fmt.Errorf("get action configuration: %w", err)
	}
	install := action.NewInstall(actionCfg)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = !cfg.skipNamespaceCreation
	install.Wait = true
	install.Timeout = 30 * time.Second
	install.Labels = map[string]string{
		LabelKeyManagedBy: LabelValueManagedBy,
	}

	log.Debug("Deploying gadget", "chartUrl", chartUrl, "releaseName", releaseName, "namespace", namespace)

	setting := cli.New()
	chartPath, err := install.ChartPathOptions.LocateChart(chartUrl, setting)
	if err != nil {
		return fmt.Errorf("locate chart: %w", err)
	}
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("load chart: %w", err)
	}

	release, err := install.RunWithContext(ctx, chart, map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("run install action: %w", err)
	}
	log.Debug("Successfully deployed Inspektor Gadget", "releaseName", release.Name, "namespace", release.Namespace)

	return nil
}

func (h *helmDeployer) Undeploy(ctx context.Context, opts ...RunOption) error {
	var cfg config
	cfg.applyOptions(opts...)
	releaseName := cfg.releaseName
	if releaseName == "" {
		releaseName = "gadget"
	}
	namespace := cfg.namespace
	if namespace == "" {
		namespace = "gadget"
	}

	deployed, err := h.IsDeployed(ctx, opts...)
	if err != nil {
		return fmt.Errorf("check if gadget is deployed: %w", err)
	}
	if !deployed {
		log.Debug("Inspektor Gadget was't deployed by this deployer, nothing to do")
		return ErrNotDeployedByDeployer
	}

	actionCfg, err := h.getActionConfig(namespace)
	if err != nil {
		return fmt.Errorf("get action configuration: %w", err)
	}
	uninstall := action.NewUninstall(actionCfg)
	uninstall.DisableHooks = true

	log.Debug("Undeploying Inspektor Gadget", "releaseName", releaseName, "namespace", namespace)

	_, err = uninstall.Run(releaseName)
	if err != nil {
		return fmt.Errorf("run uninstall action: %w", err)
	}
	log.Debug("Successfully undeployed Inspektor Gadget", "releaseName", releaseName, "namespace", namespace)

	return nil
}

func (h *helmDeployer) IsDeployed(ctx context.Context, opts ...RunOption) (bool, error) {
	var cfg config
	cfg.applyOptions(opts...)
	releaseName := cfg.releaseName
	if releaseName == "" {
		releaseName = "gadget"
	}
	namespace := cfg.namespace
	if namespace == "" {
		namespace = "gadget"
	}

	actionCfg, err := h.getActionConfig(namespace)
	if err != nil {
		return false, fmt.Errorf("get action configuration: %w", err)
	}
	get := action.NewGet(actionCfg)
	rel, err := get.Run(releaseName)
	if err != nil {
		return false, fmt.Errorf("run get action: %w", err)
	}
	for k, v := range rel.Labels {
		if k == LabelKeyManagedBy && v == LabelValueManagedBy {
			log.Debug("Inspektor Gadget is installed", "releaseName", releaseName, "namespace", namespace)
			return true, nil
		}
	}

	log.Debug("Inspektor Gadget is not installed", "releaseName", releaseName, "namespace", namespace)
	return false, nil
}

func (h *helmDeployer) getActionConfig(namespace string) (*action.Configuration, error) {
	actionConfig := action.Configuration{RegistryClient: h.registryClient}
	// Namespace is used to define scope for the Helm installation and driver is used to store release information.
	if err := actionConfig.Init(utils.KubernetesConfigFlags, namespace, os.Getenv("HELM_DRIVER"), debug); err != nil {
		return nil, fmt.Errorf("initialize action configuration: %w", err)
	}
	return &actionConfig, nil
}

func debug(format string, args ...any) {
	log.Debug(fmt.Sprintf(format, args...))
}
