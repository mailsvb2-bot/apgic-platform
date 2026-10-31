# R4 + main integration checkpoint (2026-10-19)

This branch is the **two-parent source integration** of:

- R4 branch head: `7a14d0c78125910bfb9b497045b7efadb906b186`
- current main: `3f6b3a6bcbe3d87a470dc4bf20b50f01893dcb79`
- integration merge commit: `b169d69c162a0321138013968246bb6be61c8bfa`

The combined tree retains the R4 finance code, migrations, web/native finance surfaces, operator TOTP fix, and browser regressions, plus all main changes since the R4 fork. Main and R4 concurrently changed only three paths: `deploy/staging/README.md`, `tools/mobile_android_capability_e2e.sh`, `tools/mobile_ios_capability_e2e.sh`. Independent line-level edits were retained from both parents, with no overlapping hunks. The integration head is ahead of main and not behind it.

## Hard acceptance boundary

This checkpoint is NOT a production financial certification. Keep the release gate fail-closed and **do not deploy or mark R4 RELEASED** without real signed provider/store evidence, authenticated finance order readback, installed native checks, restart/replay, and exact-HEAD CI. The database-driven finance convergence regression in `backend/internal/runtimepostgres/finance_integration_test.go` requires `APGIC_FINANCE_TEST_DATABASE_URL`, and test skip must not be mistaken for an executed real-provider journey. Realtime UCR dependent A/V acceptance remains deferred independently.

The branch exists to test the **combined** source, rather than extrapolate results from the formerly green R4-only and main-only builds.
