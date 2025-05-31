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
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/mqasimsarfraz/inspektor-gadget-mcp-server/pkg/discoverer"
	"github.com/mqasimsarfraz/inspektor-gadget-mcp-server/pkg/gadgetmanager"
	igserver "github.com/mqasimsarfraz/inspektor-gadget-mcp-server/pkg/server"
	"github.com/mqasimsarfraz/inspektor-gadget-mcp-server/pkg/tools"
)

var (
	gadgetImages     = flag.String("gadget-images", "", "comma-separated list of gadget images to use (e.g. 'trace_dns:latest,trace_open:latest')")
	gadgetDiscoverer = flag.String("gadget-discoverer", "", "gadget discoverer to use (github, artifacthub)")
	runtime          = flag.String("runtime", "grpc-k8s", "runtime to use")
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flag.Parse()

	if *gadgetDiscoverer == "" && *gadgetImages == "" {
		log.Fatal("either -gadget-images or -gadget-discoverer must be specified")
	}

	mgr, err := gadgetmanager.NewGadgetManager(*runtime)
	if err != nil {
		log.Fatalf("failed to initialize gadget manager: %v", err)
	}
	defer mgr.Close()
	registry := tools.NewToolRegistry(mgr)

	var images []string
	if gadgetImages != nil && *gadgetImages != "" {
		images = strings.Split(*gadgetImages, ",")
	} else {
		dis, err := discoverer.New(*gadgetDiscoverer)
		if err != nil {
			log.Fatalf("failed to initialize gadget discoverer: %v", err)
		}
		images, err = dis.ListImages()
		if err != nil {
			log.Fatalf("failed to list gadget images: %v", err)
		}
	}
	if err = registry.Prepare(ctx, images); err != nil {
		log.Fatalf("failed to initialize tool registry: %v", err)
	}

	igS := igserver.New("v0.0.1", registry)
	stdioS := server.NewStdioServer(igS)

	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)
		errC <- stdioS.Listen(ctx, in, out)
	}()
	select {
	case <-ctx.Done():
		log.Printf("context done, shutting down")
	case err := <-errC:
		if err != nil {
			log.Fatalf("error during server shutdown: %v", err)
		}
	}
}
