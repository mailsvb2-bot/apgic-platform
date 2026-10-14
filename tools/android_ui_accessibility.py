#!/usr/bin/env python3
"""Locate genuinely visible, enabled UIAutomator controls on a native Android screen.

The XML records real accessibility nodes. A node does not become actionable
just because it exists in the XML: tiny clipped nodes, wrong checkbox state,
disabled controls and nodes beyond the current scroll viewport are rejected.
No synthetic success label is ever introduced by this locator.
"""
from __future__ import annotations

import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

BOUNDS_RE = re.compile(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]")
DIAGNOSIS_NEGATIVE = "Это не диагноз. Диагноз поставлен: нет."
CONFIRMED_MESSAGE = "Запрос подтверждён пользователем."


def bounds(text: str) -> tuple[int, int, int, int] | None:
    match = BOUNDS_RE.fullmatch(text)
    if not match:
        return None
    x1, y1, x2, y2 = (int(x) for x in match.groups())
    return (x1, y1, x2, y2) if x2 > x1 and y2 > y1 else None


def intersection(
    a: tuple[int, int, int, int], b: tuple[int, int, int, int]
) -> tuple[int, int, int, int] | None:
    x1, y1 = max(a[0], b[0]), max(a[1], b[1])
    x2, y2 = min(a[2], b[2]), min(a[3], b[3])
    return (x1, y1, x2, y2) if x2 > x1 and y2 > y1 else None


def matches_semantics(attributes: dict[str, str], label: str) -> bool:
    desc = attributes.get("content-desc", "")
    displayed_text = attributes.get("text", "")
    if label == "native-demand-diagnosis:false":
        # Do not trust an incorrectly labelled "no diagnosis" element if
        # its actual visible text contradicts that claim.
        if displayed_text and DIAGNOSIS_NEGATIVE not in displayed_text:
            return False
        return desc == label or DIAGNOSIS_NEGATIVE in displayed_text
    if label == "native-demand-confirmed":
        if displayed_text and CONFIRMED_MESSAGE not in displayed_text:
            return False
        return desc == label or CONFIRMED_MESSAGE in displayed_text
    return desc == label


def find_accessible_point(
    xml: str, label: str, checked: str = "", action: str = "observe"
) -> tuple[int, int] | None:
    """Return screen coordinates only for a real, currently visible matching node."""
    if action not in {"observe", "tap"} or checked not in {"", "true", "false"}:
        raise ValueError("invalid accessibility query")
    hierarchy = ET.fromstring(xml)
    first_screen_node = next(hierarchy.iter("node"), None)
    if first_screen_node is None:
        return None
    screen = bounds(first_screen_node.attrib.get("bounds", ""))
    if screen is None:
        return None

    def walk(
        element: ET.Element, viewport: tuple[int, int, int, int]
    ) -> tuple[int, int] | None:
        attributes = element.attrib
        rectangle = bounds(attributes.get("bounds", ""))
        if rectangle is None:
            return None
        visible_rectangle = intersection(rectangle, viewport)
        if visible_rectangle is None:
            return None

        if matches_semantics(attributes, label) and (action == "observe" or attributes.get("enabled") == "true"):
            checkable = attributes.get("checkable") == "true"
            if not checked or (checkable and attributes.get("checked") == checked):
                if action != "tap" or attributes.get("clickable") == "true":
                    x1, y1, x2, y2 = visible_rectangle
                    # A clipped 5-pixel checkbox is NOT a legitimate touch
                    # target; however, visible 19-pixel text at y=617–636 on
                    # an actual 640-pixel display IS readable evidence.
                    minimum_height = 28 if action == "tap" else 12
                    minimum_width = 32 if action == "tap" else 12
                    if y2 - y1 >= minimum_height and x2 - x1 >= minimum_width:
                        # A clipped form control can cover the top of the
                        # confirmation button. Press its lower edge instead.
                        if action == "tap" and label == "Подтвердить темы запроса" and y2 - y1 >= 72:
                            y = y2 - 16
                        else:
                            y = (y1 + y2) // 2
                        x = (x1 + x2) // 2
                        if (screen[0] + 8 <= x <= screen[2] - 8
                                and screen[1] + 8 <= y <= screen[3] - 8):
                            return x, y

        child_viewport = (
            visible_rectangle if attributes.get("scrollable") == "true" else viewport
        )
        for child in element.findall("node"):
            found = walk(child, child_viewport)
            if found is not None:
                return found
        return None

    return walk(first_screen_node, screen)


def main() -> None:
    if len(sys.argv) != 5:
        raise SystemExit("usage: android_ui_accessibility.py XML LABEL CHECKED ACTION")
    path, label, checked, action = sys.argv[1:]
    try:
        point = find_accessible_point(Path(path).read_text(encoding="utf-8"), label, checked, action)
    except (ET.ParseError, OSError):
        # A transient UIAutomator dump or pull may fail; caller retries,
        # then reports the complete observed hierarchy as failure evidence.
        return
    if point is not None:
        print(*point)


if __name__ == "__main__":
    main()
