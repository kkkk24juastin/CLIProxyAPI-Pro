import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULES = ROOT / 'overlay/src/pro/modules'
OVERLAY_SRC = ROOT / 'overlay/src'
CUSTOMIZER = ROOT / 'apply_customizations.py'
IMPORT_PATTERN = re.compile(r"@/pro/modules/([^/'\"]+)([^'\"]*)")


class ProModuleBoundaryTests(unittest.TestCase):
    def test_cross_module_imports_use_public_index_only(self):
        violations = []
        for path in sorted(MODULES.rglob('*')):
            if path.suffix not in {'.ts', '.tsx'}:
                continue
            owner = path.relative_to(MODULES).parts[0]
            source = path.read_text(encoding='utf-8')
            for match in IMPORT_PATTERN.finditer(source):
                dependency, suffix = match.groups()
                if dependency != owner and suffix:
                    violations.append(
                        f'{path.relative_to(ROOT)} imports private path '
                        f'@/pro/modules/{dependency}{suffix}'
                    )
        self.assertEqual([], violations, '\n'.join(violations))

    def test_host_integrations_use_public_index_only(self):
        violations = []
        sources = [CUSTOMIZER]
        sources.extend(
            path
            for path in sorted(OVERLAY_SRC.rglob('*'))
            if path.suffix in {'.ts', '.tsx'} and MODULES not in path.parents
        )
        for path in sources:
            source = path.read_text(encoding='utf-8')
            for match in IMPORT_PATTERN.finditer(source):
                dependency, suffix = match.groups()
                if suffix:
                    violations.append(
                        f'{path.relative_to(ROOT)} imports private path '
                        f'@/pro/modules/{dependency}{suffix}'
                    )
        self.assertEqual([], violations, '\n'.join(violations))


if __name__ == '__main__':
    unittest.main()
