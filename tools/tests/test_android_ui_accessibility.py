from __future__ import annotations

import unittest

from tools.android_ui_accessibility import find_accessible_point


def node(
    label: str, rectangle: str, *,
    text: str = "", checked: str = "false", checkable: str = "false",
    clickable: str = "false", enabled: str = "true",
) -> str:
    import xml.sax.saxutils as escape
    def esc(value: str) -> str:
        return escape.escape(value, {'"': '&quot;'})
    return (
        f'<node content-desc="{esc(label)}" text="{esc(text)}" '
        f'bounds="{rectangle}" checked="{checked}" checkable="{checkable}" '
        f'clickable="{clickable}" enabled="{enabled}"/>'
    )


def ui(*elements: str, width: int = 320, height: int = 640) -> str:
    viewport = f"[0,0][{width},{height}]"
    return (
        f'<hierarchy rotation="0"><node bounds="{viewport}" enabled="true">'
        f'<node bounds="{viewport}" scrollable="true" enabled="true">'
        + "".join(elements)
        + '</node></node></hierarchy>'
    )


class AndroidUIAccessibilityTest(unittest.TestCase):
    def test_real_ci_failure_visible_diagnosis_at_bottom_of_display(self) -> None:
        xml = ui(node(
            "native-demand-diagnosis:false", "[24,617][296,636]",
            text="Это не диагноз. Диагноз поставлен: нет.",
        ))
        self.assertEqual(
            find_accessible_point(xml, "native-demand-diagnosis:false", action="observe"),
            (160, 626),
        )

    def test_clipped_checkbox_is_not_visible_or_actionable(self) -> None:
        xml = ui(node("Тема sleep", "[24,0][296,5]",
                      checkable="true", checked="true", clickable="true"))
        self.assertIsNone(find_accessible_point(xml, "Тема sleep", "true", "observe"))
        self.assertIsNone(find_accessible_point(xml, "Тема sleep", "true", "tap"))

    def test_full_checkbox_can_be_observed_and_tapped(self) -> None:
        xml = ui(node("Тема anxiety", "[24,530][296,578]",
                      checkable="true", checked="true", clickable="true"))
        self.assertEqual(find_accessible_point(xml, "Тема anxiety", "true", "tap"), (160, 554))
        self.assertIsNone(find_accessible_point(xml, "Тема anxiety", "false", "observe"))

    def test_confirmed_readonly_checkbox_is_visible_but_not_tappable(self) -> None:
        xml = ui(node("Тема anxiety", "[24,380][296,426]",
                      checked="false", checkable="true", clickable="true", enabled="false"))
        self.assertEqual(find_accessible_point(xml, "Тема anxiety", "false", "observe"), (160, 403))
        self.assertIsNone(find_accessible_point(xml, "Тема anxiety", "false", "tap"))

    def test_rejects_disabled_and_nonclickable_controls(self) -> None:
        self.assertIsNone(find_accessible_point(
            ui(node("Подтвердить темы запроса", "[24,400][296,448]",
                    clickable="true", enabled="false")),
            "Подтвердить темы запроса", action="tap"
        ))
        self.assertIsNone(find_accessible_point(
            ui(node("Подтвердить темы запроса", "[24,400][296,448]")),
            "Подтвердить темы запроса", action="tap"
        ))

    def test_rejects_false_safety_assertions_and_partial_labels(self) -> None:
        self.assertIsNone(find_accessible_point(
            ui(node("native-demand-diagnosis:false", "[24,500][296,520]",
                    text="Это диагноз. Диагноз поставлен: да.")),
            "native-demand-diagnosis:false"
        ))
        self.assertIsNone(find_accessible_point(
            ui(node("native-demand-notice", "[24,500][296,520]", text="Это предположение, не диагноз.")),
            "native-demand-diagnosis:false"
        ))

    def test_allows_exact_human_visible_semantics_if_label_merged(self) -> None:
        xml = ui(node("", "[24,617][296,636]", text="Это не диагноз. Диагноз поставлен: нет."))
        self.assertEqual(find_accessible_point(xml, "native-demand-diagnosis:false"), (160, 626))

    def test_adapts_to_real_device_dimensions_instead_of_hardcoded_320x640(self) -> None:
        xml = ui(node("Разобрать запрос", "[30,750][450,815]",
                      clickable="true"), width=480, height=900)
        self.assertEqual(find_accessible_point(xml, "Разобрать запрос", action="tap"), (240, 782))

    def test_confirmation_tap_uses_the_visible_center(self) -> None:
        xml = ui(node(
            "Подтвердить темы запроса", "[24,533][296,624]",
            clickable="true",
        ))
        self.assertEqual(
            find_accessible_point(xml, "Подтвердить темы запроса", action="tap"),
            (160, 578),
        )

    def test_rejects_node_outside_the_viewport(self) -> None:
        xml = ui(node("Тема sleep", "[24,700][296,748]",
                      checked="true", checkable="true", clickable="true"))
        self.assertIsNone(find_accessible_point(xml, "Тема sleep", "true", "tap"))


if __name__ == "__main__":
    unittest.main()
