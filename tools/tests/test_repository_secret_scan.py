from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from tools.repository_secret_scan import scan_repository


def write(root: Path, rel: str, value: str) -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(value, encoding="utf-8")


class RepositorySecretScanTests(unittest.TestCase):
    def test_accepts_documented_placeholders(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write(root, "config/example.yaml", 'secret: "change_me"\n')
            write(root, ".env.example", 'TOKEN=<secret>\n')
            self.assertEqual(scan_repository(root), [])

    def test_rejects_private_key_material(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write(root, "config/prod.yaml", "-----BEGIN PRIVATE KEY-----\n")
            errors = scan_repository(root)
            self.assertTrue(any("private_key" in error for error in errors))

    def test_rejects_github_token(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write(root, "scripts/deploy.sh", 'TOKEN="ghp_' + "A" * 36 + '"\n')
            errors = scan_repository(root)
            self.assertTrue(any("github_token" in error for error in errors))

    def test_rejects_embedded_basic_auth_url(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write(root, "config/runtime.yaml", "dsn: postgres://apgic:supersecret123@db.internal/apgic\n")
            errors = scan_repository(root)
            self.assertTrue(any("basic_auth_url" in error for error in errors))

    def test_ignores_generated_dependency_directories(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            write(root, "node_modules/pkg/index.js", 'const token = "ghp_' + "B" * 36 + '";\n')
            self.assertEqual(scan_repository(root), [])


if __name__ == "__main__":
    unittest.main()
