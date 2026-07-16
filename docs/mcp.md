# Querying your traces from an agent (MCP)

Otterscope exposes its data as an [MCP](https://modelcontextprotocol.io)
server, so an agent — Claude Code, the MCP Inspector, anything that speaks
MCP — can ask about its own runs: *"why did my last run fail?"*, *"what did
that tool loop cost?"*, *"is my error rate up this hour?"*

The endpoint is **`POST /mcp`** on the same port as the UI (`:8317`), using
the Streamable HTTP transport. It's read-only.

## Tools

| Tool | Does |
|---|---|
| `list_runs` | Recent runs, newest first; filter by project, status, model, service, limit. |
| `get_run` | One run by id with its full step tree, including LLM messages and tool arguments/results. |
| `get_stats` | Aggregates over a filtered window: count, error rate, p50/p95 latency, cost, tokens, assertion pass rates. |
| `list_assertions` | The eval assertions configured for a project. |

## Connect Claude Code

```sh
claude mcp add otterscope --transport http http://localhost:8317/mcp
```

Then ask it things like *"use otterscope to show my last 5 runs and tell me
which was slowest"* or *"get the steps of run <id> and explain where it went
wrong."*

## Connect any MCP client

Point it at `http://localhost:8317/mcp` with the Streamable HTTP transport.
The server negotiates protocol version `2025-11-25` (and accepts older
revisions), validates `Origin` to prevent DNS rebinding, and runs stateless
(no session header required).

## A note on exposure

The MCP endpoint has no auth — same posture as the rest of Otterscope. Keep
it on loopback (the default) or behind your own firewall/tunnel. It's read
access to your trace data; treat it like the UI.
