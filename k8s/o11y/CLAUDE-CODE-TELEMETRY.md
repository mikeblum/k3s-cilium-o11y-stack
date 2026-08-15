# Claude Code telemetry

Claude Code speaks OTLP. Point it at the collector's tailnet endpoint and its
metrics, events, and traces land in ClickHouse, queryable from Grafana.

Upstream reference: <https://code.claude.com/docs/en/monitoring-usage>

## Setup

Add to your shell profile:

```bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.<tailnet>.ts.net
```

`http/protobuf` above is not a preference — this endpoint is a Tailscale Ingress,
which proxies HTTP. The collector does listen on 4317, but reaching gRPC over the
tailnet needs the operator's Layer 3 mode (a `loadBalancerClass: tailscale`
Service, raw TCP) rather than an Ingress. Not set up here.

Start a session, send a prompt, then verify:

```sql
SELECT Timestamp, Body FROM otel.otel_logs
WHERE ServiceName = 'claude-code' ORDER BY Timestamp DESC LIMIT 20;
```

Events flush every 5s; metrics every 60s. Lower `OTEL_METRIC_EXPORT_INTERVAL`
(ms) while testing if you don't want to wait.

## Traces

Off by default. Adds a span tree per prompt — `claude_code.interaction` at the
root with `claude_code.llm_request` and `claude_code.tool` children:

```bash
export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1
export OTEL_TRACES_EXPORTER=otlp
```

## Content logging

Default is metadata only — token counts, costs, tool names, durations. Prompt
and response text are excluded until you opt in:

| Variable | Adds |
|---|---|
| `OTEL_LOG_USER_PROMPTS=1` | prompt text |
| `OTEL_LOG_ASSISTANT_RESPONSES=1` | response text (defaults to whatever `OTEL_LOG_USER_PROMPTS` is) |
| `OTEL_LOG_TOOL_DETAILS=1` | tool parameters, bash commands, skill and MCP server names |
| `OTEL_LOG_TOOL_CONTENT=1` | tool input/output in spans (requires traces) |
| `OTEL_LOG_RAW_API_BODIES=1` | full request/response JSON — very large |

The `otel_*` tables have no TTL, so anything you enable is stored indefinitely.
`OTEL_LOG_TOOL_DETAILS=1` in particular persists every bash command you run.

## Cardinality

`session.id` and `user.account_uuid` label every metric, so each session creates
new series. To trade queryability for volume:

```bash
export OTEL_METRICS_INCLUDE_SESSION_ID=false
export OTEL_METRICS_INCLUDE_ACCOUNT_UUID=false
```

Logs and traces keep `session.id` either way.

## Reference

**Metrics** (delta temporality — sum over a window, don't read the last value):
`claude_code.session.count`, `claude_code.lines_of_code.count`,
`claude_code.token.usage`, `claude_code.cost.usage`, `claude_code.commit.count`,
`claude_code.pull_request.count`, `claude_code.code_edit_tool.decision`,
`claude_code.active_time.total`.

**Events**: `claude_code.user_prompt`, `claude_code.assistant_response`,
`claude_code.tool_result`, `claude_code.tool_decision`,
`claude_code.api_request`, `claude_code.api_error`, `claude_code.api_refusal`,
`claude_code.permission_mode_changed`, `claude_code.auth`,
`claude_code.mcp_server_connection`, `claude_code.internal_error`,
`claude_code.plugin_installed`, `claude_code.plugin_loaded`.

## Other OTLP producers

Nothing here is Claude Code specific. Any OTLP client can use the same endpoint:

```bash
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.<tailnet>.ts.net
export OTEL_SERVICE_NAME=my-app
```

`OTEL_SERVICE_NAME` becomes the `ServiceName` column — set it per producer.

In-cluster workloads should use `otelcol.o11y.svc.cluster.local:4317` instead;
that path adds pod metadata.
