# Spike — OpenCode ACP: tool_call + permission shapes

Date: 2026-06-23 · opencode v1.17.9 · probe: `opencode acp --cwd /tmp/acptest`

## Goal
Capture the live JSON shapes of `tool_call`, `tool_call_update`, and the server→client
`session/request_permission` request, so Task 6 (`handleToolCall` / `handlePermissionRequest`)
maps real field names rather than guessed ones.

## Method
Drove a full ACP session (`initialize` → `session/new` → `session/prompt`) asking the agent to
run `echo hi` via its bash tool. Captured every `session/update` and any server request.

## Findings

### 1. `tool_call` (sessionUpdate) — VERIFIED
```json
{
  "sessionUpdate": "tool_call",
  "toolCallId": "call_00_Giv4awiyfFzSXVlSoaaI2534",
  "title": "bash",
  "kind": "execute",
  "status": "pending",
  "locations": [{"path": "/tmp/acptest"}],
  "rawInput": {"cwd": "/tmp/acptest"}
}
```
Field mapping for `handleToolCall`:
- tool name → `title` (e.g. `"bash"`); fallback `toolCallId`.
- input summary → `rawInput` (object).
- These match the Task 6 test `TestACPToolCallRecorded` (`title:"bash"`, `rawInput:{command:"echo hi"}`). ✅

### 2. `tool_call_update` (sessionUpdate) — VERIFIED
```json
{
  "sessionUpdate": "tool_call_update",
  "toolCallId": "call_00_Giv4awiyfFzSXVlSoaaI2534",
  "status": "in_progress",
  "kind": "execute",
  "title": "echo hi",
  "rawInput": {"command": "echo hi", "description": "Run echo hi command", "cwd": "/tmp/acptest"}
}
```
Same `toolCallId` as the originating `tool_call`. Task 6 records only on `tool_call` (not
`tool_call_update`) to avoid double-counting — confirmed correct: the update reuses the id.

### 3. `session/request_permission` — NOT OBSERVED (auto-approve)
**OpenCode v1.17.9 in ACP mode with default config AUTO-APPROVES tool execution.**
The `echo hi` bash tool ran straight through (`tool_call` → `tool_call_update` → `end_turn`)
with NO `session/request_permission` request emitted. `permreq_seen: False`.

Implications for Task 6:
- The `handlePermissionRequest` / `Respond` path is **defensive code based on the ACP spec**
  (`params.toolCall`, `params.options[].optionId`, `params.options[].kind` ∈ {allow_once, reject_once},
  client replies `{outcome:{outcome:"selected", optionId}}`). It is **not exercised by default
  opencode** and could not be captured live.
- Keep the handler (opencode CAN be configured to ask, and the seam should support it), but the
  Integrated-mode permission interrupt will **not** normally surface for opencode the way it does
  for Claude. This is acceptable: the core contract (assistant text, tool calls, end-turn,
  persistence) works without it.
- Task 6 tests for the permission path drive `handlePermissionRequest`/`Respond` directly with
  canned ACP-spec-shaped maps (unit-level), which is the right call given it can't be observed live.

## Net
Task 6 `handleToolCall`: use verified shapes above. Permission handler: implement per ACP spec,
unit-test directly, label as not-live-verified. No blocker for Tasks 5–7.
</content>
