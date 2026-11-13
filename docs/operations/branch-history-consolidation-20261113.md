# APGIC — consolidation of 47 historical branches (13 November 2026)

## Scope and safety decisions

- Baseline: `83b88a20cb992340413453d8cc2515d26d63a182`. All 47 exact branch head SHA values are pinned below.
- Consolidation strategy: Git multi-parent history integration only. Preserve the exact existing production source tree, workflows, migrations, tests, and canonical runtime. This does **not** claim that every historical branch body should be activated or that passing CI proves all product requirements.
- No new branch, no force-push, no change to operational authorization, finance, media or deletion execution. Original source from each historical branch remains retrievable from main ancestry, even after branch refs are removed.
- Old branches are not independently production-ready. Branch-local changes often predate canonical security, PostgreSQL schema, paid entitlements, or user journey guards. Merging their file trees wholesale would revert later corrections and create duplicate migrations/executors.
- The previous main has passed 11/11 CI jobs. New multi-parent merge must pass its **own** full CI before the safe branch-pruning job removes ancestor refs using SHA leases.

## Evidence and known exceptions

- 47/47 branch heads and divergences enumerated from fetched remote Git refs. All old refs are preserved as Git commit parents.
- Legacy exported Go functions and named Go/Playwright test cases were cross-checked against main. The single old lane with unique Go symbols/tests is `feat/r5-deletion-lifecycle-core-20261027`: an obsolete alternate deletion executor, old worker and provider-receipt commands. Its workflows do **not** replace the current canonical `DeletionRuntime`, `AdvanceApprovedDeletion`, `ClaimProviderErasure`, `SaveProviderErasure`, and `FinalizeVerifiedDeletion`. Original code is preserved in commit ancestry only.
- Old R8 `loadOutline` is present as a modern React `useCallback` closure; prior R5 `deleteAccount` shortcut has been replaced by signed canonical deletion intake. Identical names are not sufficient proof of equivalence, so these are recorded as deliberate architectural replacements.
- Old SQL migration numbers conflict with active migrations. The renamed active migrations are listed below. The signed-payment migration has an additional insert guard in current main, and the Persona migration differs only by a trailing blank line. Never reapply obsolete numbered files.
- Existing API routes, OpenAPI operations and browser/SQL admission checks are the source of executable truth. Branch history retention is **not** proof of external Yandex, PSP, or complete commercial production readiness.

## Renumbered active SQL migrations (old → current)

- `000042_persona_companion.sql` → `000079_persona_companion.sql`
- `000043_persona_rights_review.sql` → `000080_persona_rights_review.sql`
- `000044_r9_live_events.sql` → `000074_r9_live_events.sql`
- `000045_growth_first_party_attribution.sql` → `000075_growth_first_party_attribution.sql`
- `000046_growth_integrity_guards.sql` → `000076_growth_integrity_guards.sql`
- `000047_growth_verified_conversion_bridge.sql` → `000077_growth_verified_conversion_bridge.sql`
- `000048_growth_owned_link_lifecycle.sql` → `000078_growth_owned_link_lifecycle.sql`
- `000039_r6_instant_duty_dispatch.sql` → `000045_r6_instant_duty_dispatch.sql`
- `000049_r2_r3_signed_payment_admission.sql` → `000054_r2_r3_signed_payment_admission.sql`

## Pinned source branch heads

| Historical branch | SHA pinned for safe removal | Additional commits | Classification |
|---|---|---:|---|
| `ci/r8-real-provider-proof-on-main-20261105` | `a162cb1bce9c423baa829e1af4a985898ef3e086` | 1 | History retained; canonical source preferred |
| `feat/academy-edtech` | `e13f9889a54da4dfc262fffd27821dda0018ec5f` | 81 | History retained; canonical source preferred |
| `feat/ai-persona` | `127ce8370c6c8e7f459316b32a1f2b977eeeffe4` | 19 | Old migration numbers conflict; active renumbered migration preferred |
| `feat/client-personal` | `d5cdbfa8d3445b9ffd185d3711570ce9d653660c` | 23 | History retained; canonical source preferred |
| `feat/groups-live-events` | `3064aab22d83d360e45ec9d908a7f7dc4cfc2471` | 17 | Old migration numbers conflict; active renumbered migration preferred |
| `feat/growth-distribution` | `3adfb96b6f517cbd7cc40cc889a924151759c32d` | 25 | Old migration numbers conflict; active renumbered migration preferred |
| `feat/instant-consultation` | `f840031fdf710f570cea2d9055de14663c67b89c` | 7 | Old migration numbers conflict; active renumbered migration preferred |
| `feat/platform-readiness` | `9e7079f1c1336b3ed9b0d7eac0d70dbe9a1c3e37` | 21 | Old documentation or ancillary path; source retained in Git ancestry |
| `feat/r0-r3-dual-party-consultation-proof-20261019` | `0c4c073717c156819116fc6c22c1b162c508f1d5` | 3 | History retained; canonical source preferred |
| `feat/r2-r3-signed-payment-admission-20261026` | `a862bded6e35d45b90007e3add15a05e4d683527` | 5 | Old migration numbers conflict; active renumbered migration preferred |
| `feat/r5-canonical-deletion-state-executor-20261027` | `56e21b714789bd039b2d47e18894c70529e766b3` | 70 | History retained; canonical source preferred |
| `feat/r5-deletion-lifecycle-core-20261027` | `74bb79fbb532d6ec0b6c63a3b5b5b3197e806bd7` | 43 | Obsolete alternate executor: source retained, execution NOT enabled |
| `feat/r5-deletion-request-client-journey-20261027` | `384428630de09d335fa62cd68a00225e65ce31d8` | 8 | History retained; canonical source preferred |
| `feat/r5-durable-account-deletion-intake-20261027` | `bfbe4cf94dc9452a730c36c6a138224cb2c09a47` | 15 | History retained; canonical source preferred |
| `feat/r5-session-confirm-deletion-20261030` | `8d7f8ae3db2136b23a61538f5667759920784bb4` | 8 | History retained; canonical source preferred |
| `feat/r8-author-commerce-product-discovery-20261031` | `b2b3b906342f0d0c5c603b4a84c07c95a790455f` | 9 | History retained; canonical source preferred |
| `feat/r8-captured-evidence-all-learning-gates-20261101` | `c2d32dd6ff55c1160a86d283d81063569e7f5a93` | 7 | History retained; canonical source preferred |
| `feat/r8-cohort-integrity-20261031` | `2d58e6069baf508794af65041eb0517d34f62c5e` | 22 | History retained; canonical source preferred |
| `feat/r8-cohort-learner-journey-20261031` | `9ac6bca3c482b39dd4ba40ac92fd0306bdc4e00e` | 7 | History retained; canonical source preferred |
| `feat/r8-learner-assessment-results-20261031` | `2c53e3488b69137e93848dd9b7539a5aa371e21e` | 9 | History retained; canonical source preferred |
| `feat/r8-learner-history-export-20261031` | `7e8f88a16b04c471d60a34cc8a66b21266277bbf` | 7 | History retained; canonical source preferred |
| `feat/r8-learner-verified-access-20261101` | `fd67e72ec35d5d9cbb12d9b452273580e7d55a05` | 14 | History retained; canonical source preferred |
| `feat/r8-lesson-schedules-appeals-20261101` | `d5313a29a79e6e66c8d69e531d2ae37326c44630` | 46 | History retained; canonical source preferred |
| `feat/r8-media-grant-boundary-20261031` | `eb6c6063b47e1f819593d14363bcdc5f83789db0` | 4 | History retained; canonical source preferred |
| `feat/r8-specialist-unified-workspace-20261101` | `2b6bf6f791efa9340cbc8d7d631f9fcf28aafdd0` | 2 | History retained; canonical source preferred |
| `fix/apgic-audit-patches-20261105` | `80dc9adb238322f74194af8c37974bb7bb9be8b0` | 4 | History retained; canonical source preferred |
| `fix/deletion-residual-guard-20261030` | `c3841becb39dfba10239848c5813f8e0b46e0e62` | 3 | History retained; canonical source preferred |
| `fix/ios-staging-dns-transport-retry-20261029` | `ac5760909c8c61ca38814e507f8460126c2237e8` | 1 | History retained; canonical source preferred |
| `fix/r3-signed-payment-room-gate-release` | `ece6bf3aeb2c45fdb272f3a272bf5405ec3eaecf` | 6 | History retained; canonical source preferred |
| `fix/r5-client-session-lifecycle-20261026` | `64175101fef1592b9f7b898e83ba3b9ebf662e70` | 7 | History retained; canonical source preferred |
| `fix/r5-deletion-journey-status-20261029` | `6f7724026f64655756bcf6111b3f8b2515ba0495` | 5 | History retained; canonical source preferred |
| `fix/r5-deletion-status-unavailable-personal-20261029` | `cd5715f337d01174cede0d19bd1bcdc549577b7e` | 2 | History retained; canonical source preferred |
| `fix/r5-personal-history-concurrency-20261026` | `f24ed2175b7379aa61e76c5751510e3375392c6b` | 2 | History retained; canonical source preferred |
| `fix/r5-production-deletion-truth-20261026` | `e4382a6c51a47bbb216b9bc431008dc17248e943` | 14 | History retained; canonical source preferred |
| `fix/r5-remove-intent-identity-link-20261029` | `b2db7c8b632413ffd57258e81e90de0712d8636d` | 2 | History retained; canonical source preferred |
| `fix/r6-r2-durable-slot-exclusion-20261026` | `97238c6059f5d0014e0fc9b92b2c152868693512` | 11 | History retained; canonical source preferred |
| `fix/r6-strict-command-payload-20261026` | `e2943220419e197387f5b5b213efd736b727ffbc` | 6 | History retained; canonical source preferred |
| `fix/r8-live-workflow-nonempty-main-20261105` | `9f4e18d7003a4e489c1a820dab018e51b585dada` | 1 | History retained; canonical source preferred |
| `integrate/cross-lane-assembly-control-current-main-20261112` | `90cac22ab5d179c75745512e789f30a48a010ee0` | 3 | History retained; canonical source preferred |
| `integrate/r10-growth-current-main-20261111` | `acf3e40cbc29c92996577180be3cb9991980da8d` | 1 | History retained; canonical source preferred |
| `integrate/r5-signed-provider-evidence-current-main-20261111` | `da1e4841ae27ee63ec2656d120efbe46e1e46d91` | 3 | History retained; canonical source preferred |
| `integrate/r7-persona-current-main-20261111` | `81b7a94731ce6c2bf7ff019bdca6538a76253656` | 4 | History retained; canonical source preferred |
| `r8/academy-refund-author-metrics-proof` | `bab11faf3cd7cd23aa04ea760830967644e3ff88` | 1 | History retained; canonical source preferred |
| `r8/finance-academy-settlement-boundary` | `f703b13243ec1be2910ad80cb6d767002bc8e90b` | 4 | History retained; canonical source preferred |
| `test/r5-durable-deletion-postgres-journey-20261027` | `23dd3789c4e8613485f2bdc4c4d37435faaa850d` | 3 | History retained; canonical source preferred |
| `test/r8-video-watch-revocation-startup-20261111` | `e11f59ed6d5588d499c161b3e593d1e55cf1a10f` | 6 | History retained; canonical source preferred |
| `work/auth001-web-tenant-isolation` | `0cf5823451a76b34753da3ab73e91897dbfef75f` | 4 | Old documentation or ancillary path; source retained in Git ancestry |

## Post-merge acceptance criteria

1. New main commit has the exact original main parent and all 47 pinned branch commits as parents. Each pinned source SHA must be an ancestor of the new main.
2. `git diff` against the original main changes only this evidence file. No runtime, migration, workflow or test file may change in the consolidation merge.
3. Run the existing full GitHub CI; all 11 jobs must pass. Do not weaken checks or fake external-provider evidence.
4. The existing `prune-integrated-branches.yml` workflow must perform a fresh remote-ref inventory and use SHA lease guarded deletions. Changed remote refs are retained.
5. Only after GitHub confirms deletion and no additional heads, report that the repository has one branch. Historical source remains available by main ancestry.
