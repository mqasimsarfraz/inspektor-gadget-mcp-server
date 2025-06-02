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
	"errors"
	"fmt"
	"log/slog"
)

var ErrUnknownSource = errors.New("unknown source")

var log = slog.Default().With("component", "discoverer")

// Discoverer is used to discover available gadgets from various sources.
type Discoverer interface {
	// ListImages returns a list of available gadget images.
	ListImages() ([]string, error)
}

func New(source string) (Discoverer, error) {
	switch source {
	case SourceArtifactHub:
		return NewArtifactHubDiscoverer(), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownSource, source)
}
