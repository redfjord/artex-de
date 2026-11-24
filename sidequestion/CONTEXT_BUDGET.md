# Side-question long conversations and context budget

Fixed on 2026-09-11. The original implementation treated the character count of the request JSON directly as tokens and inherited the main task's 32K output reservation, so side questions were rejected prematurely when there was a lot of HTML, JS, or tool output.

## Open-source implementation review

- [Grok CLI side context](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/agent/agent.ts#L739): extracts fragments from the most recent user and assistant text, with a character budget of about 2000 and at most 400 characters per item. It does not maintain a continuous side Q&A history on this path.
- [Grok CLI standalone request](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/utils/side-question.ts): independent cancellation signal; output capped at 2048 tokens when the model supports it; provides no tools.
- [Grok CLI main-session compaction](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/agent/compaction.ts): estimates tokens, keeps recent content, folds new content into an older summary, and handles cross-turn truncation.
- [OpenCode session compaction](https://github.com/anomalyco/opencode/blob/b3f1a96c6dd7adeb28b36dd11add1998fc84d67b/packages/core/src/session/compaction.ts): full-request estimation, output/buffer reservation, recent content plus a rolling summary, and a tool-free summary request; that implementation defaults to a recent budget of 8000 and a summary output cap of 4096 tokens.
- [OpenCode overflow recovery](https://github.com/anomalyco/opencode/blob/b3f1a96c6dd7adeb28b36dd11add1998fc84d67b/packages/core/src/session/runner/llm.ts): attempts overflow recovery only before assistant output has started, and the recovered call does not re-enter the same overflow-recovery path.

ARTEX borrows the independent output budget, recent content plus rolling summary, and limited recovery. It keeps norma v0.3.6's structured messages and tool pairing rather than copying Grok's text excerpting; it does not write OpenCode's main-session compaction events into the ARTEX main transcript.

## Request budget and execution

- Messages use norma's UTF-8 byte estimation per content block plus a 4/3 margin; system prompt, tool schemas, and message wrapping overhead are added on top. The estimate is not an exact model token count.
- Side output defaults to at most 8192 tokens and does not exceed the output limit already set in the main configuration. The `ARTEX_BTW_MAX_OUTPUT_TOKENS` service environment variable can set a limit from 256 to 32768; it does not change the product default model or main-task parameters.
- The input budget is the context window minus the output limit and a safety margin; unknown windows use the platform default of 200K. The safety margin is 5% of the window, at least 128 and at most 8192 tokens.
- Successful Q&A pairs are loaded in batches of at most 20 by increasing sequence. At most 20 pairs are kept verbatim, with a token budget of at most 1/4 of the input budget and no more than 16K.
- Q&A beyond that is folded into a rolling summary. The summary carries the history source and context time; historical assistant answers are not equivalent to new tool evidence, and the latest main snapshot takes precedence on conflict.
- If the main context is still too long, only older messages in the copy are summarized, keeping at most the recent 8K tokens; the cut point never splits a tool call from its result. An oversized single pair goes into the summary whole.
- Summary input is split into UTF-8-safe chunks according to the actual remaining window, with an output cap of 2048 tokens; an empty summary, truncation, tool calls, or exceeding the summary budget are not written to the cache. A single side question allows at most 12 summary calls, subject to the same 120-second timeout; reaching the limit fails explicitly rather than looping indefinitely.
- If the model first reports a context overflow and has not yet emitted text or a tool call, the context is shrunk further and retried at most once; if the estimated size does not decrease, recovery stops immediately. Other model errors and partial streaming output do not trigger this recovery.
- All usage obtained—including summaries, failed attempts, and cancellation—accumulates into the same side request. When the Provider returns no usage, only zero can be recorded; estimates must not be disguised as actual usage.

## Persistence and UI

`side_question_sessions.memory` stores the old-Q&A summary, the ordinals it covers, and the main-context summary cached by snapshot identity. `side_question_requests.context_info` stores the preparation phase, the actual number of replayed pairs, summary usage, and budget estimates.

Summaries are saved only while the original request is still running and the clear version matches; Clear also purges the cache, and late writes cannot restore cleared data. A new snapshot does not reuse an old snapshot's summary. Summary fields are saved with v3 task archives; when restoring an old v3 that lacks the fields, an empty object is filled in, and v1/v2 remain compatible.

POST admits and returns the request first; preparation and compaction run in the background without holding the admission lock or a database transaction. SSE/history show the preparing, organizing Q&A, compacting copy, and answering phases; a compaction failure is saved as the failure terminal state of that side request. The frontend retains the error and restores the draft of the failed question instead of covering the input box with a floating notification; history polling no longer clears submission errors.

## Validation record

- 19/20/21/50-pair replay, retention of old conclusions across 20 pairs, and summary cache reuse after restart: automated pass.
- Very long Chinese answers and code context, chunked request budget, tool pairing, unchanged snapshot, and cache invalidation on a new snapshot: automated pass.
- Summary failure/cancellation/truncation/oversize/tool return, clear races, call limit, only one overflow recovery, and no retry on partial streams: automated pass.
- Independent PostgreSQL: paging, restart, v1/v2/v3 archives, summary-cache and budget-metadata archive/restore, old v3 missing new fields, and 20 parent sessions sharing four concurrency slots: pass.
- Go candidate service build, frontend TypeScript check, Biome check on modified components, and a Next.js production build in a separate directory: pass.
- The built-in browser used an independent UI fixture at 1280×720 and 390×844 to verify the organizing phase, summary-scope hints, draft recovery after failure, no floating error notification, no horizontal overflow, and no console errors. The temporary fixture has been removed.
- Read-only replay of existing local Worker snapshots passed the new budget check; for example, Worker #3's 293085-character snapshot is no longer misjudged and rejected by local character counting. This item made no external model calls.
- The real-conversation test that would have sent a private Worker snapshot to Grok in this change was denied by automated approval review and was not executed; it is not counted as a pass.
- At 00:37 on 2026-09-11 the local backend was restarted at the user's request, reusing the original database, data directory, and login configuration. The running file matched the candidate binary's SHA-256; `/api/health` returned normally for both the backend and the frontend proxy.

Validation commands (independent test databases only):

```sh
go test -race ./sidequestion ./db ./server -run 'TestSide|TestCheckpoint|TestSnapshot|TestBuildRequest|TestService|TestMainSide|TestTaskArchive' -count=1
go build ./cmd/artex
npx tsc --noEmit
npm run build -- --webpack
```

The frontend production build used a separate copy to avoid overwriting the current preview's `.next`. The candidate service was at `/private/tmp/artex-btw-budget-candidate`, copied to `/private/tmp/artex-btw-preview/artex` and started; the original binary was backed up as `artex.before-context-budget` in the same directory.
