# Spike — Codex app-server protocol (for the Integrated codexDriver)

Date: 2026-06-23 · codex v0.139.0 · `codex app-server` (stdio JSON-RPC 2.0, newline-delimited)

## Discovery
- Methods are slash-namespaced. Invalid method errors enumerate valid ones:
  `initialize`, `thread/start`, `thread/resume`, `thread/fork`, `thread/archive`, `thread/unsubscribe`, …
- Protocol schema dumpable via `codex app-server generate-json-schema --out DIR` (v1 = initialize only; v2 = the live "Thread" protocol).

## Verified happy-path (live capture)

### initialize
→ `{"method":"initialize","params":{"clientInfo":{"name":"worrel","version":"0.1"}}}`
← `{"result":{"userAgent":"...","codexHome":"/Users/.../.codex","platformOs":"macos"}}`

### thread/start (creates the session)
→ `{"method":"thread/start","params":{"cwd":"/abs/cwd"}}`
← `{"result":{"thread":{"id":"019ef6...","sessionId":"019ef6...","status":{"type":"idle"}, ...}}}`
Also emits notif `thread/started`, `mcpServer/startupStatus/updated`.

### turn/start (sends a user turn) — required params {threadId, input}
→ `{"method":"turn/start","params":{"threadId":"<id>","cwd":"/abs/cwd","approvalPolicy":"never",
     "input":[{"type":"text","text":"<prompt>"}]}}`
← returns IMMEDIATELY: `{"result":{"turn":{"id":"...","status":"inProgress"}}}` (the turn is NOT done yet)
`approvalPolicy` enum: `untrusted | on-failure | on-request | never` (or granular object).
`UserInput` text variant: `{"type":"text","text":"..."}`.

### Notifications during a turn (the streaming contract)
- `turn/started` — `{threadId, turn:{id, status:"inProgress"}}`
- `item/started` / `item/completed` — `{item:{type, ...}, threadId, turnId}`
  - `item.type=="userMessage"` → the echo of our own prompt (IGNORE for assistant text)
  - `item.type=="commandExecution"` → a tool/exec call. Shape:
    `{"type":"commandExecution","id":"call_...","command":"/bin/zsh -lc 'echo hi'","cwd":...,
      "status":"inProgress|completed","commandActions":[{"type":"unknown","command":"echo hi"}],
      "aggregatedOutput":...,"exitCode":...}`
  - other item types: `fileChange`, agent reasoning, etc.
- `item/agentMessage/delta` — `{threadId, turnId, itemId, delta:"ok"}` → **assistant text, streamed; accumulate by `itemId`**
- `thread/tokenUsage/updated` — `{tokenUsage:{total:{totalTokens,...}}}` (bonus: context measurement)
- `turn/completed` — `{threadId, turn:{id, status:"completed", ...}}` → **end of turn → StateAwaiting**

## Mapping to LiveSession (same contract as opencode ACP)
| Contract | codex app-server |
|---|---|
| init/start | `initialize` then `thread/start` → store `thread.id` |
| send turn | `turn/start` {threadId, input:[{type:text,text}]} |
| assistant text | accumulate `item/agentMessage/delta.delta` by `itemId` |
| tool call | `item/started` with `item.type=="commandExecution"` (name=command, summary=command) |
| end of turn | notif `turn/completed` (NOT the turn/start response) |
| user echo | `item/*` with `item.type=="userMessage"` → ignore |
| MCP worrel | pass `-c experimental_use_rmcp_client=true -c mcp_servers.worrel.url="<url>"` to `codex app-server` (same overrides the codex adapter already uses) |
| memory | prepend SystemAppend as a leading text input on the first turn (no system-prompt field) |

## Approval / permission
- v2 uses a "guardian approval review" model (`item/guardianApprovalReview/started|completed` notifications;
  `thread/approve_guardian_denied_action` request). With `approvalPolicy:"never"` (and for sandbox-safe ops
  like `echo`) **NO approval request fires** — codex auto-runs, same as opencode ACP default.
- Decision: drive `turn/start` with `approvalPolicy:"never"` for v1 of the driver (auto, matches opencode).
  Implement no permission round-trip now; the seam's `Respond` is a no-op for codex. Revisit if a
  guardian-approval round-trip is later wanted (separate spike to capture the live request shape).

## Net for the plan
Happy path + tool items + turn lifecycle are fully verified → implementable now. Permission deferred
(auto-approve), mirroring the opencode decision. No blocker.
</content>
