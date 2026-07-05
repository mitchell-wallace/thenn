# Review Workflow

Use this workflow when auditing an existing test suite.

## 1. Map The Suite Before Judging It

- Read the test commands, config, setup files, and shared helpers first.
- Identify the claimed layers: unit, integration, component, e2e, contract, smoke.
- Identify the actual seams under test: real database, real browser, in-memory harness, mocked service layer, direct component stub emission, and so on.

Do not trust folder names by default. Verify what is real.

## 2. Prioritize Trustworthiness Questions

Ask these in order:

1. Does the test exercise the boundary it claims to exercise?
2. Would a real regression at that boundary make this fail?
3. Are the assertions strong enough to prove the behavior, or are they merely suggestive?
4. Is this test meaningfully distinct from nearby tests?
5. Would a smaller number of stronger tests buy more confidence?

## 3. Review By Layer

### E2E

Treat these as the most valuable and the easiest to accidentally fake.

Raise findings when the test:

- Replaces first-party backend behavior with an in-memory harness or route intercepts.
- Uses reloads, manual state injection, or direct storage edits to bypass the user-critical transition under test.
- Asserts internal counters or fake harness state instead of browser-visible outcomes.
- Stubs shipped assets or network flows that are part of the real app contract.

Usually acceptable:

- Narrow stubs for third-party systems outside the app's ownership when the app/backend contract remains real.
- Deterministic helpers that accelerate time or polling without changing the core contract being exercised.

### Integration

Treat these as tests that should exercise multiple real modules together.

Raise findings when the test:

- Mocks nearly every collaborator and becomes a unit test with a more flattering filename.
- Stubs parent/child UI boundaries so aggressively that real wiring is no longer exercised.
- Claims to validate undo, persistence, sync, or orchestration but only checks collaborator calls.

Prefer:

- Real repositories, real local persistence, real stores, real serialization, and real module boundaries.
- A small number of scenario-focused tests rather than broad collaborator choreography checks.

### Unit

Treat these as precise tools for edge cases, contract drift, and fault isolation.

Raise findings when the test:

- Mostly proves JavaScript, Vue, Zod, or the test framework still works.
- Encodes long duplicate matrices with little increase in behavioral confidence.
- Uses shallow spot checks where the owned contract is the full object shape.
- Claims a guarantee stronger than the assertion actually proves.

Prefer:

- One invalid dimension per negative-path test where possible.
- Exact-shape assertions when the returned shape is the contract.
- Fast tests that protect branches, invariants, and failure modes the broader layers will not cover economically.

## 4. Use The Mocking Rubric Explicitly

### Gutting

Mark mocking as `gutting` when it removes the main reason the test exists.

Examples:

- "e2e" tests with an in-memory fake for auth/sync/backend behavior.
- Integration tests that replace the persistence and orchestration layers and only check calls.
- Parent view tests that rely on `$emit` from stub children instead of exercising the real interaction path.

### Murky

Mark mocking as `murky` when it may be defensible but still hides important flow risk.

Examples:

- Stubbing a sub-boundary that carries part of the user interaction.
- Mocking network or storage at a layer that claims to exercise retry, persistence, or convergence logic.
- Assertions that only prove a call happened, not that the right user-observable state emerged.

### Pragmatic

Mark mocking as `pragmatic` when it isolates noise while preserving the main contract.

Examples:

- Freezing time or UUID generation in unit tests.
- Replacing an unrelated analytics emitter while still exercising the real core behavior.
- Stubbing a truly external third-party boundary in a higher-layer test when the product-owned contract remains real.

## 5. Look For Low-Signal Test Smells

Raise or trim when you see:

- Removal-only or deprecation-only tests that mostly prove something is gone, unless the compatibility risk is real.
- Exact heading/copy/order assertions where wording is not the product requirement.
- Tautological tests that merely restate the component or function signature.
- Large duplicate matrices that repeat the same logic with renamed data.
- Tests titled as performance, N+1, or resilience guards that never observe performance, query count, retries, or resilience behavior.

## 6. Prefer Findings That Explain Risk

Good findings answer:

- What is the test claiming to protect?
- Why does the current setup fail to protect it?
- What regression could slip through?
- What stronger test or rewrite would better protect it?

## 7. Report Format

Default to:

- Severity
- File and line references
- Why the current test is weak, misleading, redundant, or actually sound
- What to do instead

Lead with the sharpest issues first. Keep summaries brief.
