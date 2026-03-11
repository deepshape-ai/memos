# MCP Server

This package implements a [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server embedded in the Memos HTTP process. It exposes memo operations as MCP tools, making Memos accessible to any MCP-compatible AI client (Claude Desktop, Cursor, Zed, etc.).

## Endpoint

```
POST /mcp   (tool calls, initialize)
GET  /mcp   (optional SSE stream for server-to-client messages)
```

Transport: [Streamable HTTP](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports) (single endpoint, MCP spec 2025-03-26).

## Authentication

Every request must include a Personal Access Token (PAT):

```
Authorization: Bearer <your-PAT>
```

PATs are long-lived tokens created in Settings → My Account → Access Tokens. Short-lived JWT session tokens are not accepted. Requests without a valid PAT receive `HTTP 401`.

## Tools

All tools are prefixed with `memos_` for discoverability and to avoid conflicts with other MCP servers. All tools include annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`) to help clients understand tool behavior.

### Memo Tools

| Tool | Description | Required params | Optional params |
|---|---|---|---|
| `memos_list_memos` | List memos | — | `page_size` (int, max 100), `page`, `state`, `order_by_pinned`, `filter` (CEL expression) |
| `memos_get_memo` | Get a single memo | `name` | — |
| `memos_search_memos` | Full-text search | `query` | — |
| `memos_create_memo` | Create a memo | `content` | `visibility` |
| `memos_update_memo` | Update content or visibility | `name` | `content`, `visibility`, `pinned`, `state` |
| `memos_delete_memo` | Delete a memo | `name` | — |
| `memos_list_memo_comments` | List comments on a memo | `name` | — |
| `memos_create_memo_comment` | Add a comment to a memo | `name`, `content` | — |

**`name`** is the memo resource name, e.g. `memos/abc123`.

**`visibility`** accepts `PRIVATE` (default), `PROTECTED`, or `PUBLIC`.

**`filter`** accepts CEL expressions supported by the memo filter engine, e.g.:
- `content.contains("keyword")`
- `visibility == "PUBLIC"`
- `has_task_list`

### Daily Log Tools

| Tool | Description | Required params | Optional params |
|---|---|---|---|
| `memos_save_daily_log` | Create or update today's daily log | `date`, `content` | — |
| `memos_get_daily_log` | Get a daily log by date | `date` | `creator` |
| `memos_list_daily_logs` | List daily logs with date range | — | `start_date`, `end_date`, `creator`, `page_size` (max 100), `page` |

**`date`** is always `YYYY-MM-DD`.

**`creator`** is a user resource name, e.g. `users/1`. Defaults to the authenticated user.

**`content`** must use `.plan`-style line prefixes: `* ` (done), `+ ` (to-do), `- ` (note), `? ` (question). No indentation allowed.

Daily logs are always `PROTECTED` visibility and follow a one-per-user-per-day rule. Only today's log (within a 36-hour window) can be saved; past logs are immutable.

### Tag Tools

| Tool | Description | Required params | Optional params |
|---|---|---|---|
| `memos_list_tags` | List all tags with memo counts | — | — |

## Response Format

All tools return JSON responses with consistent structure:

- **Timestamps**: Both Unix (`create_time`, `update_time`) and ISO 8601/RFC3339 (`create_time_iso`, `update_time_iso`) formats
- **Pagination**: Includes `has_more`, `page`, `page_size` for list operations
- **Error messages**: Include actionable suggestions for resolution

## Connecting Claude Code

```bash
claude mcp add --transport http memos http://localhost:5230/mcp \
  --header "Authorization: Bearer <your-PAT>"
```

Use `--scope user` to make it available across all projects:

```bash
claude mcp add --scope user --transport http memos http://localhost:5230/mcp \
  --header "Authorization: Bearer <your-PAT>"
```

## Package Structure

| File | Responsibility |
|---|---|
| `mcp.go` | `MCPService` struct, constructor, route registration |
| `tools_memo.go` | Tool registration and memo tool handlers |
| `tools_tag.go` | Tag tool registration and handler |
| `tools_daily_log.go` | Daily log tool registration and three daily-log handlers |
| `prompts.go` | MCP prompts: capture, daily_log, review |
