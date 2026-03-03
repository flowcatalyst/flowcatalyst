# @flowcatalyst/mcp-server

MCP (Model Context Protocol) server that gives AI agents read-only access to your FlowCatalyst event types, schemas, subscriptions, and generated code.

Works with Claude Code, Claude Desktop, Cursor, Windsurf, and any MCP-compatible client.

## What it provides

| Tool | Description |
|---|---|
| `list_event_types` | List event types with optional filters (status, application, subdomain, aggregate) |
| `get_event_type` | Get full event type detail including all schema versions |
| `get_schema` | Get the raw JSON Schema for a specific version (defaults to CURRENT) |
| `generate_code` | Generate TypeScript, PHP, Python, or Java code from a schema |
| `list_subscriptions` | List webhook subscriptions (scoped to your client) |
| `get_subscription` | Get subscription details |

Resources are also available at `flowcatalyst://event-types`, `flowcatalyst://event-types/{id}`, `flowcatalyst://subscriptions`, and `flowcatalyst://subscriptions/{id}`.

## Prerequisites

You need a **service account** with the `AI Agent Read-Only` role (`PLATFORM_AI_AGENT_READONLY`). This grants read-only access to event types and subscriptions.

Create one in the FlowCatalyst admin UI under **IAM > Service Accounts**, then assign the `AI Agent Read-Only` role.

## Installation

```bash
npm install -g @flowcatalyst/mcp-server
```

Or run directly with npx:

```bash
npx @flowcatalyst/mcp-server
```

## Configuration

Set three environment variables:

```bash
export FLOWCATALYST_URL=https://your-instance.flowcatalyst.io
export FLOWCATALYST_CLIENT_ID=svc_abc123
export FLOWCATALYST_CLIENT_SECRET=your_secret
```

## Setup by client

### Claude Code

```bash
claude mcp add flowcatalyst -- npx @flowcatalyst/mcp-server
```

Then set the environment variables in your shell profile, or use Claude Code's env configuration.

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "flowcatalyst": {
      "command": "npx",
      "args": ["@flowcatalyst/mcp-server"],
      "env": {
        "FLOWCATALYST_URL": "https://your-instance.flowcatalyst.io",
        "FLOWCATALYST_CLIENT_ID": "svc_abc123",
        "FLOWCATALYST_CLIENT_SECRET": "your_secret"
      }
    }
  }
}
```

### Cursor

Add to `.cursor/mcp.json` in your project root:

```json
{
  "mcpServers": {
    "flowcatalyst": {
      "command": "npx",
      "args": ["@flowcatalyst/mcp-server"],
      "env": {
        "FLOWCATALYST_URL": "https://your-instance.flowcatalyst.io",
        "FLOWCATALYST_CLIENT_ID": "svc_abc123",
        "FLOWCATALYST_CLIENT_SECRET": "your_secret"
      }
    }
  }
}
```

### Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "flowcatalyst": {
      "command": "npx",
      "args": ["@flowcatalyst/mcp-server"],
      "env": {
        "FLOWCATALYST_URL": "https://your-instance.flowcatalyst.io",
        "FLOWCATALYST_CLIENT_ID": "svc_abc123",
        "FLOWCATALYST_CLIENT_SECRET": "your_secret"
      }
    }
  }
}
```

## HTTP transport

For hosted deployment, run with the `--http` flag:

```bash
FLOWCATALYST_URL=https://... \
FLOWCATALYST_CLIENT_ID=svc_abc123 \
FLOWCATALYST_CLIENT_SECRET=secret \
npx @flowcatalyst/mcp-server --http
```

This starts a streamable HTTP server on port 3100 (override with `PORT` env var). The MCP endpoint is at `/mcp`.

## Example usage

Once connected, an AI agent can:

- **Explore your event catalog**: "What event types are registered for the `billing` application?"
- **Read schemas**: "Show me the JSON Schema for the `order:fulfillment:order:created` event type"
- **Generate code**: "Generate a TypeScript interface for the `user:iam:user:created` event"
- **Check subscriptions**: "What subscriptions are configured and what events do they listen to?"
- **Build integrations**: "Generate PHP DTOs for all event types in the `payments` subdomain so I can handle these webhooks"

## Authentication

The server authenticates using OAuth2 client credentials flow against your FlowCatalyst instance's OIDC token endpoint (`{FLOWCATALYST_URL}/oidc/token`). Tokens are cached and automatically refreshed before expiry.

## Development

```bash
cd clients/mcp-server
npm install
npm run build    # tsc
npm run dev      # tsc --watch
```

## License

Apache-2.0
