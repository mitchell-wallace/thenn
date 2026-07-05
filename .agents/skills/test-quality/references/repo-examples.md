# Repo Examples

Use these examples when working in this repository or when you need concrete illustrations of the skill's heuristics.

## Example: E2E Against A Fake Backend

Files:

- `apps/frontend/tests/e2e/helpers/authSync.ts`
- `apps/frontend/tests/e2e/helpers/statefulAuthSync.ts`
- `apps/frontend/tests/e2e/settings-sync.spec.ts`
- `apps/frontend/tests/e2e/data-management.spec.ts`

Why it matters:

- The browser is real, but major first-party backend behavior is replaced with an in-memory harness.
- The tests often assert harness state or request counters directly.
- This is useful as frontend-against-harness coverage, but weaker than the `e2e` label suggests.

Case study: auth and sync release confidence

- `installMockAuthAndSync` and `createStatefulAuthAndSyncHarness` are not third-party mocks. They replace this app's own auth, settings, data, and sync APIs.
- For specs that claim auth/session, cross-device sync, conflict convergence, remote data clearing, or account isolation, this is usually `gutting` mock behavior: it removes the backend boundary the test appears to prove.
- These harnesses can still be valuable for deterministic frontend states, guest-local flows, offline UI setup, and fast browser regression coverage.
- The confidence fix is not "delete the tests"; it is to add or promote required real-backend e2e for the highest-risk contracts, then label harness-backed coverage honestly.
- A green harness-backed e2e run should be described as "browser flows pass against the frontend test harness," not "auth/sync works end to end."

## Example: "Integration" Tests That Behave Like Units

Files:

- `apps/frontend/tests/integration/memoryDuplicateWarning.test.ts`
- `apps/frontend/tests/integration/prayerBulkArchiveUndo.test.ts`
- `apps/frontend/tests/integration/prayerActionConvergence.test.ts`

Why it matters:

- These tests mock most of the meaningful collaborators.
- They still provide signal, but the real integration boundaries are no longer being exercised.
- They are good candidates for either renaming/reclassification or rebuilding around more real seams.

## Example: Parent View Tests Bypassing The Real Interaction Path

Files:

- `apps/frontend/tests/components/views/HomeView.test.ts`
- `apps/frontend/tests/components/views/ListView.bulk.test.ts`
- `apps/frontend/tests/components/views/PrayerDetailView.test.ts`

Why it matters:

- Direct `$emit(...)` calls from stubs prove parent handlers react.
- They do not prove the real child UI still emits the right events under user interaction.
- This is often a `murky` or `gutting` mock depending on the test's claims.

## Example: Tests Claiming More Than They Measure

Files:

- `apps/backend/tests/functional/sync/hierarchical_notes.spec.ts`
- `apps/backend/tests/unit/services/entity_crud_service.spec.ts`

Why it matters:

- The N+1 tests are named like performance/query guards but do not observe query count.
- The positive-path validation test says it proves an existence check, but it never asserts the lookup happened.
- These are classic cases where test names oversell the guarantee.

## Example: Weak Negative-Path Schema Tests

Files:

- `packages/shared/tests/schemas/prayer.test.ts`
- `packages/shared/tests/schemas/memory.test.ts`
- `packages/shared/tests/schemas/sync_dto.test.ts`

Why it matters:

- Several cases only check `safeParse(...).success === false`.
- Some payloads pack multiple invalid fields into a single example.
- This can keep tests green even when the intended failure path changes.

## Example: Brittle Low-Signal Assertions

Files:

- `apps/frontend/tests/components/views/SettingsView.test.ts`

Why it matters:

- Exact heading-copy checks and HTML-order assertions create churn quickly.
- They are worth keeping only when wording or ordering is itself a requirement.
