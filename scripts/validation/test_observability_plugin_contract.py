import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
HOST_STORE = (
    ROOT
    / "cliproxyapi-pro-core"
    / "patches"
    / "sources"
    / "internal"
    / "pro"
    / "observability"
    / "store.go"
)
PLUGIN_STORE = (
    ROOT
    / "cliproxyapi-pro-plugins"
    / "pro-observability"
    / "store.go"
)
PLUGIN_SOURCE = ROOT / "cliproxyapi-pro-plugins" / "pro-observability" / "plugin.go"


def table_columns(path: Path, table: str) -> list[str]:
    source = path.read_text()
    match = re.search(
        rf"create table if not exists {re.escape(table)}\s*\((.*?)\)\s*`",
        source,
        flags=re.IGNORECASE | re.DOTALL,
    )
    if match is None:
        raise AssertionError(f"{path} does not define {table}")
    columns: list[str] = []
    for raw_line in match.group(1).splitlines():
        line = raw_line.strip().rstrip(",")
        if not line:
            continue
        name = line.split(maxsplit=1)[0].lower()
        if name in {"primary", "unique", "constraint", "check", "foreign"}:
            continue
        columns.append(name)
    return columns


class ObservabilityPluginContractTest(unittest.TestCase):
    def test_usage_events_schema_matches_current_host(self) -> None:
        host_columns = table_columns(HOST_STORE, "usage_events")
        plugin_columns = table_columns(PLUGIN_STORE, "usage_events")
        self.assertEqual(host_columns, plugin_columns)
        self.assertEqual(len(host_columns), len(set(host_columns)))

    def test_usage_summary_schema_matches_current_host(self) -> None:
        self.assertEqual(
            table_columns(HOST_STORE, "usage_summary"),
            table_columns(PLUGIN_STORE, "usage_summary"),
        )

    def test_plugin_has_no_frontend_resource_or_menu(self) -> None:
        source = PLUGIN_SOURCE.read_text()
        self.assertNotIn("ResourceRoute", source)
        self.assertNotIn("Resources:", source)
        self.assertNotRegex(source, r"\bMenu\s*:")
        self.assertIn('MigrationMode: "shadow-writer-disabled"', source)


if __name__ == "__main__":
    unittest.main()
