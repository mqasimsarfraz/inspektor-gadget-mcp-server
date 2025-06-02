# Inspektor Gadget MCP Server

Inspektor Gadget MCP Server allows you to debug and inspect Kubernetes clusters using different gadgets.

# Features

- AI interface to troubleshoot and workload your workloads on Kubernetes clusters.
- Helps understand and summarize output of gadgets.
- Discover gadgets from Artifact Hub.

# Installation

## Prerequisites
- `kubeconfig` file with access to the Kubernetes cluster.
- [Inspektor Gadget installed ](https://inspektor-gadget.io/docs/latest/reference/install-kubernetes) in the cluster.

## Using MCP Client

### VS Code

[VS Code supports MCP](https://code.visualstudio.com/docs/copilot/chat/mcp-servers), allowing you to connect to the MCP server. There are two ways you can configure the MCP server in VS Code:

#### Cetralized User Settings

You can set the MCP server in your user settings. Open the command palette (Ctrl+Shift+P) and select "Preferences: Open User Settings (JSON)". Then add the following configuration:

```
...
    "mcp": {
        "inspektor-gadget": {
            "type": "stdio",
            "command": "docker",
            "args": [
                "run",
                "-i",
                "--rm",
                "-v", "~/.kube/config:/kubeconfig:ro",
                "-e", "KUBECONFIG=/kubeconfig",
                "ghcr.io/mqasimsarfraz/inspektor-gadget-mcp-server:latest",
                "-gadget-discoverer=artifacthub"
            ]
        }
    },
...
```

#### Workspace Settings

You can also set the MCP server in your workspace settings by writing `.vscode/mcp.json` in your project/workspace directory. The content of the file should be similar to the following:

```json
{
  "servers": {
    "inspektor-gadget": {
      "type": "stdio",
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-v",
        "~/.kube/config:/kubeconfig:ro",
        "-e",
        "KUBECONFIG=/kubeconfig",
        "ghcr.io/mqasimsarfraz/inspektor-gadget-mcp-server:latest",
        "-gadget-discoverer=artifacthub"
      ]
    }
  }
}
```
