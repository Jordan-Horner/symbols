#!/usr/bin/env python3
import json
import re
import sys
from pathlib import Path


def fail(msg: str) -> None:
    print(f"release-metadata check failed: {msg}", file=sys.stderr)
    sys.exit(1)


def read_main_version(main_go: Path) -> str:
    text = main_go.read_text(encoding="utf-8")
    m = re.search(r'var\s+version\s*=\s*"([^"]+)"', text)
    if not m:
        fail(f"could not find version in {main_go}")
    return m.group(1)


def main() -> None:
    repo = Path(__file__).resolve().parents[1]
    main_go = repo / "main.go"
    server_json = repo / "server.json"

    cli_version = read_main_version(main_go)
    data = json.loads(server_json.read_text(encoding="utf-8"))

    server_version = data.get("version", "")
    if server_version != cli_version:
        fail(f"server.json version ({server_version}) != main.go version ({cli_version})")

    packages = data.get("packages", [])
    if not packages:
        fail("server.json packages is empty")

    pkg = packages[0]
    pkg_version = pkg.get("version", "")
    if pkg_version != cli_version:
        fail(f"server.json packages[0].version ({pkg_version}) != main.go version ({cli_version})")

    identifier = pkg.get("identifier", "")
    expected_prefix = f"/download/v{cli_version}/"
    if expected_prefix not in identifier:
        fail(f"packages[0].identifier does not include {expected_prefix}: {identifier}")

    allowed_suffixes = (
        "syms_darwin_arm64.tar.gz",
        "syms-mcp_darwin_arm64.tar.gz",
    )
    if not identifier.endswith(allowed_suffixes):
        fail(
            "packages[0].identifier must target one of "
            f"{', '.join(allowed_suffixes)} (got: {identifier})"
        )

    sha = pkg.get("fileSha256", "")
    if not re.fullmatch(r"[0-9a-f]{64}", sha):
        fail("packages[0].fileSha256 must be 64 lowercase hex chars")

    print(f"release-metadata check passed for version {cli_version}")


if __name__ == "__main__":
    main()
