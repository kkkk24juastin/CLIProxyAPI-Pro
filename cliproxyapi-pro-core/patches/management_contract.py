import ast
import re
from pathlib import Path


ANTIGRAVITY_USER_AGENT_PLACEHOLDER = "__MANAGEMENT_ANTIGRAVITY_USER_AGENT__"


def _typescript_string_constant(source: str, name: str) -> str:
    pattern = re.compile(
        rf"export\s+const\s+{re.escape(name)}\s*=\s*(?P<literal>'(?:\\.|[^'\\])*'|\"(?:\\.|[^\"\\])*\")\s*;"
    )
    match = pattern.search(source)
    if match is None:
        raise ValueError(f"Management constant {name} was not found")
    value = ast.literal_eval(match.group("literal"))
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"Management constant {name} is empty")
    return value.strip()


def antigravity_user_agent(management_root: Path) -> str:
    constants_path = management_root / "src/utils/quota/constants.ts"
    if not constants_path.is_file():
        raise ValueError(f"Management quota constants not found: {constants_path}")
    source = constants_path.read_text(encoding="utf-8")

    if not re.search(
        r"['\"]User-Agent['\"]\s*:\s*ANTIGRAVITY_USER_AGENT\b",
        source,
    ):
        raise ValueError("Management Antigravity request headers no longer use ANTIGRAVITY_USER_AGENT")

    direct = re.search(
        r"export\s+const\s+ANTIGRAVITY_USER_AGENT\s*=\s*"
        r"(?P<literal>'(?:\\.|[^'\\])*'|\"(?:\\.|[^\"\\])*\")\s*;",
        source,
    )
    if direct is not None:
        value = ast.literal_eval(direct.group("literal"))
        if isinstance(value, str) and value.strip():
            return value.strip()
        raise ValueError("Management ANTIGRAVITY_USER_AGENT is empty")

    if not re.search(
        r"export\s+const\s+ANTIGRAVITY_USER_AGENT\s*=\s*buildAntigravityUserAgent\(\s*\)\s*;",
        source,
    ):
        raise ValueError("Management ANTIGRAVITY_USER_AGENT construction is unsupported")

    builder_match = re.search(
        r"export\s+const\s+buildAntigravityUserAgent\s*=\s*(?P<body>.*?)"
        r"\n\s*export\s+const\s+ANTIGRAVITY_USER_AGENT\b",
        source,
        re.DOTALL,
    )
    if builder_match is None:
        raise ValueError("Management buildAntigravityUserAgent implementation was not found")
    template_match = re.search(
        r"=>\s*`(?P<template>[^`]+)`\s*;",
        builder_match.group("body"),
    )
    if template_match is None:
        raise ValueError("Management Antigravity user-agent template was not found")

    values = {
        "version": _typescript_string_constant(source, "ANTIGRAVITY_CLI_VERSION"),
        "clientName": _typescript_string_constant(source, "ANTIGRAVITY_CLIENT_NAME"),
    }
    platform_match = re.search(
        r"export\s+const\s+ANTIGRAVITY_CLIENT_PLATFORM\s*=\s*\{(?P<body>.*?)\}\s*as\s+const\s*;",
        source,
        re.DOTALL,
    )
    if platform_match is None:
        raise ValueError("Management ANTIGRAVITY_CLIENT_PLATFORM was not found")
    platform = platform_match.group("body")
    for source_key, template_key in (("osType", "osType"), ("arch", "arch")):
        match = re.search(
            rf"\b{source_key}\s*:\s*(?P<literal>'(?:\\.|[^'\\])*'|\"(?:\\.|[^\"\\])*\")",
            platform,
        )
        if match is None:
            raise ValueError(f"Management Antigravity platform {source_key} was not found")
        value = ast.literal_eval(match.group("literal"))
        if not isinstance(value, str) or not value.strip():
            raise ValueError(f"Management Antigravity platform {source_key} is empty")
        values[template_key] = value.strip()

    template = template_match.group("template")
    placeholders = set(re.findall(r"\$\{([^}]+)\}", template))
    if placeholders != set(values):
        raise ValueError(
            "Management Antigravity user-agent template placeholders changed: "
            + ", ".join(sorted(placeholders))
        )
    for key, value in values.items():
        template = template.replace("${" + key + "}", value)
    if "${" in template or not template.strip():
        raise ValueError("Management Antigravity user-agent could not be resolved")
    return template.strip()
