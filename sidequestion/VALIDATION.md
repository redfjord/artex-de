# `/btw` validation record

Date: 2026-09-10. Branch: `codex/btw-side-question`. Baseline: `8dae851b9b622f2ff2631f332fde9719d0b16fba`.

An independent PostgreSQL test database and data directory were used; real model credentials were injected only into the isolated test environment, were not written to code or this record, and the product default model was not changed. Go 1.26.3, norma v0.3.6, Next.js 16.2.9.

The actual model conversations, response objects, engineering assertions, and the raw Qwen review text are stored in [validation-2026-09-10.json](validation-2026-09-10.json), which contains no API credentials.

## Engineering checks

| Scope | Result | Evidence |
| --- | --- | --- |
| Structured messages, deep copy of tool arguments | Pass | `TestCheckpointDeepCopyAndBoundaries` |
| Summary/compaction requests do not overwrite; complete replies and terminal states are published; partial replies excluded | Pass | `TestCheckpointDeepCopyAndBoundaries`, `TestSnapshotExcludesPartialStreamAndSelectsPoolMember` |
| Actual model pool member identity | Pass | `TestSnapshotExcludesPartialStreamAndSelectsPoolMember` |
| Tool pairing, 20-pair replay, budget trimming, and over-limit errors | Pass | `TestBuildRequestCompactionToolPairingAndBudget` |
| Main/side parallelism and bidirectional cancellation isolation | Pass | Blocking Provider, `TestMainSideConcurrencyAndIndependentCancellation` |
| No tool execution, streaming/non-streaming, usage retained on failure | Pass | `TestServiceNoToolsAndUsageOnFailure` |
| Real norma ChatAgent + local Read tool, main transcript/activity isolation | Pass | `TestSideActualChatCheckpointToolResultAndTranscriptIsolation`, streaming and non-streaming subtests |
| Persistence, paging, idempotency, partial answer retained across restart | Pass | `TestSideHistoryIdempotencyPagingAndRecovery` |
| Clear racing late writers, parent-resource deletion, version comparison | Pass | `TestSideClearLateWritersAndDeletedParent` |
| MainAgent/Worker archiving and restoration, v1/v2/v3 | Pass | `TestSideTaskArchiveVersions` |
| Three parent endpoints, auth, resource ownership, Worker logical deletion | Pass | `TestSideHTTPGlobalLimitTaskWorkerAndDeletion`, `TestSideCheckpointPersistsBeforeAdmissionAndRestart` |
| Side questions on a busy main session, independent SSE reconnect/drop, cancel, clear | Pass | `TestSideHTTPBusyIsolationClearAndReconnect` |
| 1 per parent session / 4 globally concurrent | Pass | The two `TestSideHTTP…` cases |
| Snapshot persisted before submission, follow-up after restart, old sessions cannot forge a snapshot | Pass | `TestSideCheckpointPersistsBeforeAdmissionAndRestart` |
| Reject continuation after the cached configuration is deleted or the model changes | Pass | `TestSideRejectsDeletedOrChangedCachedProfile` |
| Cancel before archiving and wait for the final answer and usage to persist | Pass | `TestSideTaskDrainPersistsBeforeArchive` |
| Record usage once and attribute it to the side question when a streaming consumer cancels early | Pass | `TestSideUsageRecordedOnceOnConsumerCancellation` |
| Worker/deadline run contexts restored after restart continue publishing new snapshots | Pass | `TestSideRestoredWorkerRuntimePublishesNewCheckpoint` |
| Race check on relevant packages | Pass | Command below |
| TypeScript and production build | Pass | `npx tsc --noEmit`, `npm run build` |
| Biome on new frontend modules | Pass | `biome check`, 3 new modules |

After configuring `ARTEX_PG_DSN` for a separate disposable database, the automated checks can be reproduced (do not point it at a production database):

```sh
go test -race ./agent ./db ./server ./sidequestion ./llmrec ./llmpool \
  -run 'Test(Side|Checkpoint|Snapshot|BuildRequest|Service|MainSide|CaptureRun|TaskArchive|CompleteForwards|StopIntent|CancelIntent)' -count=1
cd web
npx tsc --noEmit
npx biome check src/lib/side-questions.ts src/hooks/use-side-questions.ts src/components/side-question-workspace.tsx
npm run build
```

The full Go regression is not all green: two existing tests in the `server` package fail during temporary-directory cleanup, both reporting `TempDir RemoveAll … directory not empty`:

- `TestInheritedActivityDetailAndRelationDeletion`
- `TestTaskMetadataPatchReturnsRenameAndPin`

Rerunning the `server` package in the same isolated environment after exporting the source from the unchanged baseline above also reproduces these two cleanup failures. The baseline run additionally failed the target-node-count assertion in `TestCoreTaskLifecyclePG`; the final modified `server` regression did not have that assertion failure. Other packages passed, and the side-question cases and race checks passed in this change. Baseline issues were not marked as passing for this acceptance, and existing assertions were not modified to hide problems.

The Next.js build output shows the pre-existing multi-lockfile / workspace-root inference warnings; the build completed and all pages were generated successfully.

## Browser checks

Codex In-app Browser was used against an independent local Go service and Next.js dev server. The following manual automated actions were completed on desktop and a 390 × 844 narrow viewport, checking screenshots and browser logs:

- Typing `/btw` while ordinary chat is running shows the main content and the side panel simultaneously; the desktop panel works normally.
- Consecutive follow-ups; stopping a side question keeps the generated portion; the main flow continues.
- Closing the panel lets the request continue; reopening restores the completed answer; after a page refresh, an empty `/btw` restores history.
- The narrow-viewport Drawer's input, buttons, history, and close action work with no horizontal overflow.
- Clear uses a confirmation dialog; after clearing the history disappears while the main transcript and snapshot remain.
- A task's MainAgent and two Workers were asked separately and switched between; Agent tabs and histories did not cross-talk.
- A blocking local model fixture kept a Worker running; `/btw` was submitted from the Worker's main input box, and after stopping the side question the Worker still showed live execution and its own pause button while the side question saved its partial answer.
- Browser error/warning logs were empty.

Controllable fixtures were used to verify concurrency timing precisely without depending on real-model output speed. During debugging, two Worker runtime checks failed to form a valid concurrency window (the task had ended / the answer ended early); after fixing the fixture they were redone and passed. Those initial attempts are not recorded as valid passes.

## Real-model conversations

`grok-4.6` was probed first, via the OpenAI-compatible endpoint `http://127.0.0.1:12580/tingly/openai`. The probe returned HTTP 200 with model name `grok-4.6` and `READY`, in 2.82 seconds. Because the preferred option was available, the Tingly `glm` and Zhipu `glm-5.3` fallback chains were not enabled; those two fallback services were not validated in this run.

| Scenario | Actual result |
| --- | --- |
| Ask about asset, goal, and marker while the main session is running | Returned `redhaze.top`, the homepage read plus a summary of the goal, and `BTW-REAL-0910`; side question completed in 16.97 seconds |
| Ask for tool evidence after the main session finished reading the homepage | Correctly cited WebFetch 200, curl redirects 301 → 302 → 200, and the page title; 7.24 seconds |
| Side question asks Bash to create a test file | Refused to execute; the target file was not created; 7.74 seconds |
| A side question after completion does not change the main context | Main transcript SHA-256 stayed consistent with the main activity record; side tool executions were 0 |
| Follow-up after a real stop/restart of the Go service | Kept the previous 3 side-question history entries and answered the asset, marker, and title directly from the persisted snapshot, without rerunning the main Agent |
| New session using the Grok non-streaming configuration | Correctly answered the asset and `ATOMIC-0910`; returned and saved usage: input 11734, output 138, cache_read 11520 |

For the asset case, the main session used WebFetch and Bash/curl to read the public homepage; the landing page was `https://id.redhaze.top/home`, titled "RedHaze Technology RedHaze Group · Global Integrated Group Portal". Bash cached the response in a local test file; no write was performed against the remote. This fact was verified separately from "the side question executed no tools".

Main transcript checksum: `e7e61f135a4a120954b539f357e8c4205d7d5cd7460dcaf3dc0fd066463e1d00`.

**Usage limitation:** Tingly's Grok streaming responses did not return usage. A separate direct request with `stream_options.include_usage=true` verified this: HTTP 200, 12 data frames, 0 usage frames. Thus a 0 in streaming tests means the endpoint supplied no usage, and must not be read as zero billing. Non-streaming usage and the fixture's failure/cancel usage were saved correctly.

## Qwen review

The review model was `qwen-flash`, OpenAI-compatible endpoint `https://dashscope.aliyuncs.com/compatible-mode/v1`, HTTP 200. It was given the first three real side-question conversations, the main-session tool evidence, and the engineering assertions; it returned `verdict: accept`, `concerns: []`, judging that the answers were consistent with the asset, marker, and page-read evidence, and that the side question's tool refusal complied with the constraints. Review usage: prompt 6625, completion 312, total 6937.

This Qwen review scope does not include the later-added service restart and non-streaming tests. Qwen's summary of "no writes" was too broad: the main session's curl did create a local response temporary file, as recorded explicitly above. Concurrency, zero tool execution, and transcript isolation were judged by engineering assertions; the model review only assisted in evaluating answer quality.
