import ast
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
GENERATOR_PATHS = (
    ROOT / 'cliproxyapi-pro-core/patches/apply_upstream_patches.py',
    ROOT / 'cliproxyapi-pro-management/apply_customizations.py',
)


def loaded_names(node: ast.AST) -> set[str]:
    return {
        child.id
        for child in ast.walk(node)
        if isinstance(child, ast.Name) and isinstance(child.ctx, ast.Load)
    }


def unreachable_top_level_functions(path: Path) -> set[str]:
    tree = ast.parse(path.read_text(), filename=str(path))
    functions = {
        node.name: node
        for node in tree.body
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))
    }
    function_names = set(functions)
    dependencies = {
        name: loaded_names(node) & function_names
        for name, node in functions.items()
    }
    roots: set[str] = set()
    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        roots.update(loaded_names(node) & function_names)

    reachable: set[str] = set()
    pending = list(roots)
    while pending:
        name = pending.pop()
        if name in reachable:
            continue
        reachable.add(name)
        pending.extend(dependencies[name] - reachable)
    return function_names - reachable


class SourceReachabilityTest(unittest.TestCase):
    def test_generator_top_level_functions_are_reachable(self) -> None:
        for path in GENERATOR_PATHS:
            with self.subTest(path=path.relative_to(ROOT)):
                unreachable = sorted(unreachable_top_level_functions(path))
                self.assertEqual(
                    [],
                    unreachable,
                    f'unreachable top-level functions in {path.relative_to(ROOT)}: '
                    f'{", ".join(unreachable)}',
                )


if __name__ == '__main__':
    unittest.main()
