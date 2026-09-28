from __future__ import annotations

import unittest

from tools.mobile_sbom import SBOMError, build_cyclonedx_sbom, component_ref, validate_sbom


PACKAGE = {
    "name": "@apgic/mobile",
    "version": "0.0.1",
    "dependencies": {
        "react": "19.3.0",
        "react-native": "0.87.1",
    },
    "devDependencies": {
        "typescript": "^5.9.0",
    },
}


def installed_tree() -> dict:
    return {
        "name": "@apgic/mobile",
        "version": "0.0.1",
        "dependencies": {
            "react": {"version": "19.3.0"},
            "react-native": {
                "version": "0.87.1",
                "dependencies": {
                    "metro-runtime": {"version": "0.83.1"},
                },
            },
            "typescript": {"version": "5.9.3"},
        },
    }


class MobileSBOMTests(unittest.TestCase):
    def test_generates_cyclonedx_with_transitive_components_and_edges(self) -> None:
        sbom = build_cyclonedx_sbom(PACKAGE, installed_tree())
        self.assertEqual(validate_sbom(sbom, PACKAGE), [])
        refs = {component["bom-ref"] for component in sbom["components"]}
        self.assertIn(component_ref("react", "19.3.0"), refs)
        self.assertIn(component_ref("react-native", "0.87.1"), refs)
        self.assertIn(component_ref("metro-runtime", "0.83.1"), refs)

        rn_ref = component_ref("react-native", "0.87.1")
        rows = {row["ref"]: row["dependsOn"] for row in sbom["dependencies"]}
        self.assertEqual(rows[rn_ref], [component_ref("metro-runtime", "0.83.1")])

    def test_output_is_deterministic_for_same_graph(self) -> None:
        first = build_cyclonedx_sbom(PACKAGE, installed_tree())
        second = build_cyclonedx_sbom(PACKAGE, installed_tree())
        self.assertEqual(first, second)

    def test_rejects_missing_declared_direct_dependency(self) -> None:
        tree = installed_tree()
        del tree["dependencies"]["react-native"]
        with self.assertRaisesRegex(SBOMError, "missing declared dependencies"):
            build_cyclonedx_sbom(PACKAGE, tree)

    def test_rejects_npm_graph_problems(self) -> None:
        tree = installed_tree()
        tree["problems"] = ["missing peer dependency"]
        with self.assertRaisesRegex(SBOMError, "contains problems"):
            build_cyclonedx_sbom(PACKAGE, tree)

    def test_ignores_uninstalled_optional_placeholder(self) -> None:
        tree = installed_tree()
        tree["dependencies"]["bufferutil"] = {}
        sbom = build_cyclonedx_sbom(PACKAGE, tree)
        refs = {component["bom-ref"] for component in sbom["components"]}
        self.assertFalse(any("bufferutil" in ref for ref in refs))

    def test_rejects_installed_component_without_resolved_version(self) -> None:
        tree = installed_tree()
        tree["dependencies"]["react"] = {"path": "node_modules/react"}
        with self.assertRaisesRegex(SBOMError, "no resolved version"):
            build_cyclonedx_sbom(PACKAGE, tree)


if __name__ == "__main__":
    unittest.main()
