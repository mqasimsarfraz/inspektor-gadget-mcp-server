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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/inspektor-gadget/ig-mcp-server/pkg/server"

	"github.com/inspektor-gadget/ig-mcp-server/pkg/discoverer"
	"github.com/inspektor-gadget/ig-mcp-server/pkg/gadgetmanager"
	"github.com/inspektor-gadget/ig-mcp-server/pkg/tools"
)

// This variable is used by the "version" command and is set during build
var version = "undefined"

var (
	// MCP server configuration
	transport     = flag.String("transport", "stdio", fmt.Sprintf("transport to use (%s)", strings.Join(server.SupportedTransports, ", ")))
	transportHost = flag.String("transport-host", "localhost", "host for the transport")
	transportPort = flag.String("transport-port", "8080", "port for the transport")
	// Inspektor Gadget configuration
	environment                   = flag.String("environment", "kubernetes", "environment to use (kubernetes or linux)")
	linuxRemoteAddress            = flag.String("linux-remote-address", "", "Comma-separated list of remote address (gRPC) to connect. If not set 'unix:///var/run/ig/ig.socket' is used.")
	gadgetImages                  = flag.String("gadget-images", "", "comma-separated list of gadget images to use (e.g. 'trace_dns:latest,trace_open:latest')")
	gadgetDiscoverer              = flag.String("gadget-discoverer", "", "gadget discoverer to use (artifacthub)")
	artifactHubDiscovererOfficial = flag.Bool("artifacthub-official", false, "use only official gadgets from Artifact Hub")
	// Server configuration
	logLevel    = flag.String("log-level", "", "log level (debug, info, warn, error)")
	versionFlag = flag.Bool("version", false, "print version and exit")
)

var log = slog.Default().With("component", "ig-mcp-server")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flag.Parse()

	if *versionFlag {
		log.Info("Inspektor Gadget MCP Server", "version", version)
		os.Exit(0)
	}

	if *gadgetDiscoverer == "" && *gadgetImages == "" {
		logFatal("either -gadget-images or -gadget-discoverer must be specified")
	}

	if *logLevel != "" {
		l, err := parseLogLevel(*logLevel)
		if err != nil {
			logFatal("invalid log level", "error", err)
		}
		slog.SetLogLoggerLevel(l)
	}

	if *environment != "kubernetes" && *environment != "linux" {
		logFatal("invalid environment, must be 'kubernetes' or 'linux'")
	}

	if *linuxRemoteAddress != "" && *environment != "linux" {
		logFatal("linux-remote-address can only be set when environment is 'linux'")
	}

	mgr, err := gadgetmanager.NewGadgetManager(*environment, *linuxRemoteAddress)
	if err != nil {
		logFatal("failed to create gadget manager", "error", err)
	}
	defer mgr.Close()
	registry := tools.NewToolRegistry(mgr, *environment)

	var images []string
	if gadgetImages != nil && *gadgetImages != "" {
		images = strings.Split(*gadgetImages, ",")
	} else {
		var opts []discoverer.Option
		if *artifactHubDiscovererOfficial {
			opts = append(opts, discoverer.WithArtifactHubOfficialOnly(true))
		}
		dis, err := discoverer.New(*gadgetDiscoverer, opts...)
		if err != nil {
			logFatal("failed to create gadget discoverer", "error", err)
		}
		images, err = dis.ListImages()
		if err != nil {
			logFatal("failed to list gadget images", "error", err)
		}
	}

	srv := server.New(version, registry)
	if err = registry.Prepare(ctx, images); err != nil {
		logFatal("failed to prepare tool registry", "error", err)
	}

	go func() {
		if err = srv.Start(*transport, *transportHost, *transportPort); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("failed to start server", "error", err)
		}
	}()

	<-ctx.Done()
	log.Info("Received shutdown signal, shutting down server")
	if err = srv.Shutdown(ctx); err != nil {
		logFatal("failed to shutdown server", "error", err)
	}
}

func logFatal(msg string, args ...any) {
	log.Error(msg, args...)
	os.Exit(1)
}

func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("invalid log level: %s", level)
}
