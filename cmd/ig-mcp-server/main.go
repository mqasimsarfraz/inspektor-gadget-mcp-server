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
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/inspektor-gadget/ig-mcp-server/pkg/discoverer"
	"github.com/inspektor-gadget/ig-mcp-server/pkg/gadgetmanager"
	igserver "github.com/inspektor-gadget/ig-mcp-server/pkg/server"
	"github.com/inspektor-gadget/ig-mcp-server/pkg/tools"
)

var (
	gadgetImages                  = flag.String("gadget-images", "", "comma-separated list of gadget images to use (e.g. 'trace_dns:latest,trace_open:latest')")
	gadgetDiscoverer              = flag.String("gadget-discoverer", "", "gadget discoverer to use (artifacthub)")
	artifactHubDiscovererOfficial = flag.Bool("artifacthub-official", false, "use only official gadgets from Artifact Hub")

	runtime  = flag.String("runtime", "grpc-k8s", "runtime to use")
	logLevel = flag.String("log-level", "", "log level (debug, info, warn, error)")
)

var log = slog.Default().With("component", "ig-mcp-server")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flag.Parse()

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

	mgr, err := gadgetmanager.NewGadgetManager(*runtime)
	if err != nil {
		logFatal("failed to create gadget manager", "error", err)
	}
	defer mgr.Close()
	registry := tools.NewToolRegistry(mgr)

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

	igS := igserver.New("v0.0.1", registry)
	stdioS := server.NewStdioServer(igS)

	if err = registry.Prepare(ctx, images); err != nil {
		logFatal("failed to prepare tool registry", "error", err)
	}

	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)
		errC <- stdioS.Listen(ctx, in, out)
	}()
	select {
	case <-ctx.Done():
		log.Info("shutting down server gracefully")
	case err := <-errC:
		if err != nil {
			logFatal("server error", "error", err)
		}
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
