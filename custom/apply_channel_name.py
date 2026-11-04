#!/usr/bin/env python3
"""Show API-key channel group names in the CLIProxyAPI Pro management panel.

Applied on top of an upstream Cli-Proxy-API-Management-Center checkout *after*
the Pro customizations (cliproxyapi-pro-management/apply.sh).

Changes:
  1. services/api/transformers.ts
     v8 config returns per-key `auth_index`, but the parser only reads
     `auth-index`, so request logs can never be matched back to a channel.
     Read both spellings.
  2. pro/modules/monitoring/features/sourceResolver.ts
     Use the v8 group `name` as the display name for API-key providers
     (prefix still wins when set).
  3. features/providers/components/ProviderResourceTable.tsx
     Show the group `name` as the primary label of API-key provider rows,
     with the key preview underneath.

Every replacement asserts its expected hit count, so upstream drift fails
loudly instead of silently producing a different panel. Re-running on an
already patched tree is a no-op.
"""
import sys
from pathlib import Path

MARKER = "channel-name-patch"


def patch(root: Path, rel: str, edits: list[tuple[str, str, int]]) -> None:
    path = root / rel
    text = path.read_text(encoding="utf-8")
    if MARKER in text:
        print(f"skip (already patched): {rel}")
        return
    for old, new, expected in edits:
        hits = text.count(old)
        if hits != expected:
            sys.exit(f"{rel}: expected {expected} occurrence(s) of {old!r}, found {hits}")
        text = text.replace(old, new)
    text = f"// {MARKER}: applied by custom/apply_channel_name.py\n" + text
    path.write_text(text, encoding="utf-8")
    print(f"patched: {rel}")


def main() -> None:
    if len(sys.argv) != 2:
        sys.exit("usage: apply_channel_name.py /path/to/Cli-Proxy-API-Management-Center")
    root = Path(sys.argv[1]).resolve() / "src"

    patch(root, "services/api/transformers.ts", [
        ("normalizeAuthIndex(record?.['auth-index'])",
         "normalizeAuthIndex(record?.['auth-index'] ?? record?.['auth_index'])", 3),
        ("normalizeAuthIndex(provider['auth-index'])",
         "normalizeAuthIndex(provider['auth-index'] ?? provider['auth_index'])", 1),
    ])

    patch(root, "pro/modules/monitoring/features/sourceResolver.ts", [
        ("const buildProviderIdentityKey = (type: string, index: number) => `${type}:${index}`;",
         "const buildProviderIdentityKey = (type: string, index: number) => `${type}:${index}`;\n\n"
         "const resolveGroupName = (item: { source?: ProviderKeyConfig['source'] }) => {\n"
         "  const name = item.source?.group?.name;\n"
         "  return typeof name === 'string' ? name.trim() : '';\n"
         "};", 1),
        ("items: Array<{ apiKey?: string; prefix?: string; authIndex?: string }>;",
         "items: Array<{\n"
         "      apiKey?: string;\n"
         "      prefix?: string;\n"
         "      authIndex?: string;\n"
         "      source?: ProviderKeyConfig['source'];\n"
         "    }>;", 1),
        ("displayName: item.prefix?.trim() || `${label} #${index + 1}`,",
         "displayName: item.prefix?.trim() || resolveGroupName(item) || `${label} #${index + 1}`,", 1),
    ])

    # The Pro core forces management.panel-github-repository back to the upstream
    # repository on every start, so the panel's "check update" button would
    # replace this patched panel with the upstream one. Panel updates come from
    # the image instead; the button reports "unchanged" without calling the API.
    patch(root, "pro/system/ManagementVersionTileExtension.tsx", [
        ("      const result = await checkManagementPanelUpdate();\n",
         "      // Patched panel is shipped in the image; never fetch the upstream panel.\n"
         "      const result: Awaited<ReturnType<typeof checkManagementPanelUpdate>> = await Promise.resolve({\n"
         "        status: 'custom-image',\n"
         "        updated: false,\n"
         "        sha256: '',\n"
         "      });\n", 1),
    ])

    patch(root, "features/providers/components/ProviderResourceTable.tsx", [
        ("        <span className={styles.primaryName}>{r.apiKeyPreview ?? '—'}</span>\n"
         "        {r.authIndex ? <span className={styles.primarySub}>auth: {r.authIndex}</span> : null}\n",
         "        <span className={styles.primaryName}>{r.name ?? r.apiKeyPreview ?? '—'}</span>\n"
         "        {r.name ? (\n"
         "          <span className={styles.primarySub}>{r.apiKeyPreview ?? '—'}</span>\n"
         "        ) : r.authIndex ? (\n"
         "          <span className={styles.primarySub}>auth: {r.authIndex}</span>\n"
         "        ) : null}\n", 1),
    ])


if __name__ == "__main__":
    main()
