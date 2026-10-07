from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "client_credential_guard",
    ROOT / "tools/client_credential_guard.py",
)
guard = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(guard)


class ClientCredentialGuardTest(unittest.TestCase):
    def test_default_roots_cover_real_web_and_mobile_trees(self) -> None:
        relative = {path.relative_to(ROOT).as_posix() for path in guard.CLIENT_ROOTS}
        self.assertEqual(relative, {"apps/web", "apps/mobile"})
        self.assertTrue(all(path.is_dir() for path in guard.CLIENT_ROOTS))

    def test_surface_roots_use_shared_mobile_tree_for_ios_and_android(self) -> None:
        self.assertEqual(guard.roots_for_surface("WEB"), (ROOT / "apps/web",))
        self.assertEqual(guard.roots_for_surface("IOS"), (ROOT / "apps/mobile",))
        self.assertEqual(guard.roots_for_surface("ANDROID"), (ROOT / "apps/mobile",))
        with self.assertRaises(ValueError):
            guard.roots_for_surface("DESKTOP")

    def test_rejects_public_and_server_secret_refs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bad.ts").write_text(
                'const a = process.env.SERVICE_PRINCIPAL_SECRET;\n'
                'const b = process.env.NEXT_PUBLIC_CONNECTOR_CLIENT_SECRET;\n',
                encoding="utf-8",
            )
            errors = guard.scan_client_roots((root,))
            self.assertEqual(len(errors), 2)

    def test_allows_public_non_secret_configuration(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "ok.ts").write_text(
                'const apiBase = process.env.NEXT_PUBLIC_API_BASE_URL;\n',
                encoding="utf-8",
            )
            self.assertEqual(guard.scan_client_roots((root,)), [])


if __name__ == "__main__":
    unittest.main()
