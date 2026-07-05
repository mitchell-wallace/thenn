---
name: test-quality
description: Review, critique, and design automated tests with a confidence-over-coverage philosophy. Use when Codex needs to audit a test suite for over-mocking, shallow assertions, low-signal or redundant coverage, misleading layer labels, weak end-to-end realism, or brittle implementation-detail tests; and also when Codex needs to write or refactor tests toward higher-confidence e2e, real integration coverage, and high-signal unit tests. Works broadly for modern TS/web apps and includes repo-local examples for this project.
---

# Test Quality

## Overview

Use this skill to review existing tests or design new ones with an explicit bias toward trustworthiness, realism, and signal. Prefer fewer honest tests over many flattering ones.

## Core Stance

- Prefer confidence over coverage.
- Prefer real e2e and real integration tests at the highest layer that can cheaply prove the behavior.
- Use unit tests to cover edge cases, protect contracts from drift, and provide faster fault isolation when something breaks.
- Prefer exact-shape assertions when the shape is the contract and the payload is still readable.
- Trim low-signal tests that mostly prove the framework, restate the implementation, or multiply near-duplicate cases without increasing confidence.
- Always raise mocking as a review point, then assess its impact instead of treating every mock as equally bad.

## Choose The Mode

### Review Mode

Use this mode when the user asks to review, audit, critique, or improve an existing test suite.

1. Map the intended layers first: unit, integration, component, e2e, contract, smoke.
2. Inspect harnesses, helpers, setup files, and shared fixtures before trusting the test names.
3. Ask of each test: "What real regression would make this fail?"
4. Prioritize findings where the claimed layer and the actual behavior diverge.
5. Report findings first, then recommendations.

Read [reviewing.md](references/reviewing.md) for the full workflow and smell catalog.

### Writing Mode

Use this mode when the user asks to write tests, improve coverage, or replace weak tests with stronger ones.

1. Start at the highest honest layer that can prove the behavior.
2. Add lower-layer tests only when they buy edge-case coverage, contract protection, or failure resolution.
3. Prefer one strong real-flow test over several mocked tests that all restate the same path.
4. Use strict, readable assertions instead of arbitrary spot checks when verifying owned contracts.
5. Avoid vanity expansion: do not multiply tests just to enumerate obvious permutations.

Read [writing.md](references/writing.md) for layer selection, assertion guidance, and density heuristics.

## Mocking Review Rubric

Always call out mocking. Then classify it:

- `Gutting`: the mock removes the main contract or boundary the test claims to exercise.
- `Murky`: the mock may be pragmatic, but it hides enough of the flow that important regressions could slip through.
- `Pragmatic`: the mock isolates noise or nondeterminism while preserving the key behavior and keeping assertions user-meaningful.

Escalate severity based on impact, not ideology. A stubbed clock in a unit test is not the same problem as an e2e test backed by an in-memory fake server.

## Output Expectations

### For Review Work

- Lead with findings ordered by severity.
- Include file references and explain why the current test is weak, misleading, redundant, or stronger than it first appears.
- Pair each finding with a recommendation that would improve confidence, not just increase test count.
- If no meaningful findings are present, say so clearly and note any residual risks or suite blind spots.

### For Writing Work

- Recommend or implement the smallest set of tests that gives real confidence.
- Explain why each proposed layer exists.
- Prefer observable outcomes over collaborator-call assertions unless the collaborator call is itself the contract.

## Repo Notes

When working in this repository:

- Use `pnpm`.
- Before debugging failing tests, read `docs/testing/TROUBLESHOOTING.md`.
- Write tests one file at a time and run that file before moving on.
- Use TypeScript in `apps/frontend/src/` and `apps/frontend/tests/`.

Read [repo-examples.md](references/repo-examples.md) for local examples of high-signal and low-signal test patterns from this codebase.
