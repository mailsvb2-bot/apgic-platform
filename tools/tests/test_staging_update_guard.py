from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "staging_update_guard",
    ROOT / "tools/staging_update_guard.py",
)
guard = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(guard)


class StagingUpdateGuardTest(unittest.TestCase):
    def run_guard_with(self, text: str) -> None:
        original = guard.SCRIPT
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "update-staging.sh"
            path.write_text(text, encoding="utf-8")
            guard.SCRIPT = path
            try:
                guard.main()
            finally:
                guard.SCRIPT = original

    def test_current_updater_contract_passes(self) -> None:
        text = (ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8")
        self.run_guard_with(text)

    def test_rejects_missing_ledger_bootstrap_backup_condition(self) -> None:
        text = (ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8")
        broken = text.replace(
            'if [[ "$ledger_exists" != "t" ]] || (("${#new_migrations[@]}" > 0)); then',
            'if (("${#new_migrations[@]}" > 0)); then',
        )
        self.assertNotEqual(text, broken)
        with self.assertRaises(SystemExit):
            self.run_guard_with(broken)

    def test_rejects_missing_target_environment_preflight(self) -> None:
        text = (ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8")
        broken = text.replace(
            'echo "target deployment requires missing environment key: $target_key" >&2',
            'echo "missing key" >&2',
        )
        self.assertNotEqual(text, broken)
        with self.assertRaises(SystemExit):
            self.run_guard_with(broken)

    def test_rejects_missing_target_updater_reexec(self) -> None:
        text = (ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8")
        broken = text.replace(
            'APGIC_UPDATE_REEXEC=1 exec bash "$REPO_ROOT/deploy/staging/update-staging.sh" "$TARGET_SHA"',
            'echo "skip target updater"',
        )
        self.assertNotEqual(text, broken)
        with self.assertRaises(SystemExit):
            self.run_guard_with(broken)

    def test_rejects_backup_after_ledger_reconcile(self) -> None:
        text = (ROOT / "deploy/staging/update-staging.sh").read_text(encoding="utf-8")
        start = text.index('if [[ "$backup_required" == "true" ]]')
        end = text.index('echo "=== Reconcile migration ledger ==="')
        backup_block = text[start:end]
        broken = text[:start] + text[end:] + "\n" + backup_block
        with self.assertRaises(SystemExit):
            self.run_guard_with(broken)


if __name__ == "__main__":
    unittest.main()
