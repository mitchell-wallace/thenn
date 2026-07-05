# Writing Workflow

Use this workflow when writing new tests or replacing weak ones.

## 1. Start With The Highest Honest Layer

Choose the first layer that can cheaply prove the behavior:

- Use `e2e` for user-critical flows, frontend/backend contracts, auth/session/cookie behavior, sync convergence, and persistence that must survive a real session boundary.
- Use `integration` for real orchestration across multiple local modules, repositories, stores, persistence, serialization, or rendering boundaries.
- Use `unit` for edge cases, branch protection, invariants, schema validation, exact output contracts, and faster diagnosis.

Do not start at unit level out of habit if the real risk lives at a higher layer.

## 2. Add Lower Layers Only For A Reason

Add unit or integration tests beneath a higher-layer test when they buy one of these:

- Edge-case coverage that would be awkward or noisy at the higher layer
- Exact contract protection against drift
- Faster diagnosis when the broad flow breaks

If the lower-layer test does not buy one of those, it is probably vanity coverage.

## 3. Prefer Exact Contracts Over Arbitrary Spot Checks

When the code owns a response shape, schema, derived object, or state transition:

- Prefer strict assertions on the full shape when the payload is still readable.
- Split large objects into named expectations only when that improves clarity.
- Avoid asserting two or three "important" fields arbitrarily if the contract is broader and stable enough to check fully.

Use spot checks when:

- The payload is intentionally large and only a subset matters to the behavior.
- The rest of the structure is already protected elsewhere.
- Full-shape assertions would create more churn than signal.

## 4. Keep E2E Real

Prefer:

- Real backend, real cookies/session flows, real asset loading, and real persistence
- User-visible assertions
- Fresh-session or cross-device validation when the behavior claims persistence or sync

Be cautious with:

- In-memory backend harnesses
- Route intercepts for first-party APIs
- Reload-based shortcuts that bypass reconnect/recovery logic
- Direct localStorage/IndexedDB manipulation to force state

## 5. Keep Integration Honest

Prefer:

- Real stores with real repositories where feasible
- Real local persistence or serialization boundaries
- Real component wiring when the interaction boundary matters

Avoid:

- Calling methods directly in loops when the product behavior is really an undo flow, toast flow, or modal flow
- Naming tests as integration when nearly everything meaningful is mocked

## 6. Keep Unit Tests Sharp

Prefer unit tests for:

- Edge conditions
- Validation failures
- Exact return shapes
- Drift-sensitive transformations
- Failure-mode and retry logic
- Narrow invariants that should break loudly when changed by accident

Avoid:

- Large permutations that all prove the same branch
- Tests of framework defaults or language primitives
- Shallow assertions that under-describe the contract

## 7. Trim Duplicate Density

When choosing between many small tests and fewer stronger ones:

- Prefer the smallest set that gives real confidence.
- Collapse duplicate tests that hit the same branch with cosmetic input changes.
- Keep one representative test per distinct behavior, not per sentence of implementation.

Ask:

- Does this test protect a distinct regression?
- Would its failure teach something new?
- If I remove it, what confidence actually disappears?

## 8. Repo Conventions For This Project

When writing tests here:

- Use `pnpm`.
- Read `docs/testing/TROUBLESHOOTING.md` before deep debugging of failing tests.
- Write tests one file at a time and run that file before moving on.
- Use TypeScript in frontend source and frontend tests.

Typical commands:

```bash
pnpm --filter moved-by-the-word-app exec vitest --run tests/unit/path/to/file.test.ts
pnpm --filter moved-by-the-word-app exec vitest --run tests/integration/path/to/file.test.ts
pnpm --filter moved-by-the-word-app exec playwright test tests/e2e/path/to/file.spec.ts
pnpm --filter @mbtw/backend test functional --files tests/functional/path/to/file.spec.ts
```

## 9. Default Deliverable

When the user asks for stronger tests, default to:

- A brief explanation of why each chosen layer exists
- The smallest credible set of tests
- Strong observable assertions
- Minimal mocking, with any remaining mocks justified explicitly
