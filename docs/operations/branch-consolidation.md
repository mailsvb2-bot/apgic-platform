# Branch consolidation inventory (read-only)

This tool is part of the canonical integration workflow, **not** a second runtime,
release authority, or automatic merge engine.

Run from a freshly fetched checkout:

```bash
git fetch --prune origin '+refs/heads/*:refs/remotes/origin/*'
python3 tools/branch_consolidation.py --base origin/main --output evidence/branch-functional-inventory.json
python3 -m unittest discover -s tools/tests -p 'test_branch_consolidation.py' -v
```

The report pins full branch SHA values, counts ancestry inclusion, and highlights
source files absent from the current baseline and numbered migration collisions.
These are **review prompts**, not proof that a function is missing. A different
file name, cherry-picked implementation, or replacement module may already
supply the same behavior. Never merge an old migration or competing runtime
because it appears in the report.

For each divergent lane review the current implementation, request/DB/effect
ownership, functional diff, API/contract and exact migration ledger. Integrate
one coherent behavior block (code, schema, clients, tests and operational proof),
run independent real journeys, and merge only after successful CI. Never
delete source branches before explicitly establishing equivalence or accepted
supersession. CI success is not production authorization.
