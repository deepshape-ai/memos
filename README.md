# Memos

A self-hosted, open-source note-taking service. Fork of [usememos/memos](https://github.com/usememos/memos) with additional features: Daily Log and MCP (Model Context Protocol) server integration.

## Fork Features

### Daily Log

A dedicated memo type for time-series daily notes.

- Each date has exactly one log entry, accessed via `/daily-log` in the UI
- Structured content format with `.plan`-style line prefixes (`* `, `+ `, `- `, `? `)
- Content validation enforcing prefix formatting and no indentation
- Immutability: only the current day's log (within a 36-hour window) can be edited; past logs are read-only
- Always workspace-visible (PROTECTED visibility)
- Timezone-aware date boundaries
- Calendar view showing daily log activity
- REST API: `PUT/GET/DELETE /api/v1/daily-logs/:date`, `GET /api/v1/daily-logs` (with date range filtering and pagination)

### MCP Server

An embedded [Model Context Protocol](https://modelcontextprotocol.io) server that exposes memo operations to AI assistants (Claude Code, Cursor, Zed, etc.).

- Endpoint: `POST /mcp` (tool calls), `GET /mcp` (SSE stream)
- Authentication via Personal Access Tokens (Bearer header)
- Tools: `list_memos`, `get_memo`, `search_memos`, `create_memo`, `update_memo`, `delete_memo`, `list_tags`
- Resources: memo resources indexed by UID
- Prompts: `capture` (quick memo creation), `review` (search and summarize)

Connection example:

```bash
claude mcp add --transport http memos http://localhost:5230/mcp \
  --header "Authorization: Bearer <your-PAT>"
```

See [server/router/mcp/README.md](server/router/mcp/README.md) for full documentation.

## Upstream Features

- Timeline-first UI for instant capture
- Self-hosted with full data ownership, Markdown storage, zero telemetry
- Single Go binary, lightweight Docker image
- SQLite, MySQL, or PostgreSQL backends
- REST and gRPC APIs
- MIT license

## Quick Start

### Docker

```bash
docker run -d \
  --name memos \
  -p 5230:5230 \
  -v ~/.memos:/var/opt/memos \
  neosmemo/memos:stable
```

Open `http://localhost:5230` to start.

### Build from Source

```bash
# Backend
go run ./cmd/memos --port 8081

# Frontend (in web/)
pnpm install
pnpm dev
```

See the upstream [installation guide](https://usememos.com/docs/deploy) for Docker Compose, Kubernetes, and binary options.

## Tech Stack

| Layer    | Technology                                         |
|----------|----------------------------------------------------|
| Backend  | Go 1.25, Echo v5, Connect RPC + gRPC-Gateway       |
| Frontend | React 18, TypeScript, Vite, Tailwind CSS v4         |
| API      | Protocol Buffers, REST, gRPC                        |
| Database | SQLite, MySQL, PostgreSQL                           |
| Build    | Multi-stage Docker (Alpine 3.21), multi-arch        |

## License

MIT License. See [LICENSE](LICENSE).

Upstream project: [usememos/memos](https://github.com/usememos/memos)
