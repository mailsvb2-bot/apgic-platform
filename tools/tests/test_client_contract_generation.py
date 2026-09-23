import copy
import unittest

from tools.generate_client_contract import ContractGenerationError, render_contract


class ClientContractGenerationTests(unittest.TestCase):
    def base_document(self):
        return {
            "info": {"version": "1.2.3"},
            "paths": {
                "/b": {"post": {"operationId": "beta"}},
                "/a": {"get": {"operationId": "alpha"}},
            },
            "components": {
                "schemas": {
                    "Example": {
                        "type": "object",
                        "required": ["value"],
                        "properties": {
                            "optional": {"type": "boolean"},
                            "value": {"type": "string"},
                        },
                    },
                    "Mode": {"type": "string", "enum": ["A", "B"]},
                }
            },
        }

    def test_generation_is_stable_across_mapping_order(self) -> None:
        left = self.base_document()
        right = copy.deepcopy(left)
        right["paths"] = {
            "/a": right["paths"]["/a"],
            "/b": right["paths"]["/b"],
        }
        right["components"]["schemas"] = {
            "Mode": right["components"]["schemas"]["Mode"],
            "Example": right["components"]["schemas"]["Example"],
        }
        self.assertEqual(render_contract(left), render_contract(right))

    def test_duplicate_operation_id_is_rejected(self) -> None:
        document = self.base_document()
        document["paths"]["/b"]["post"]["operationId"] = "alpha"
        with self.assertRaises(ContractGenerationError):
            render_contract(document)


if __name__ == "__main__":
    unittest.main()
