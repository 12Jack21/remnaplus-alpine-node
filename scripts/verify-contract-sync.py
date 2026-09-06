#!/usr/bin/env python3
"""Verify an official node checkout against the reviewed local contract baseline."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


def fail(message: str) -> int:
    print(f"contract sync verification failed: {message}", file=sys.stderr)
    return 1


def load_json(path: Path) -> dict[str, object]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{path} must contain an object")
    return value


def output(*args: str) -> str:
    return subprocess.check_output(args, text=True).strip()


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {Path(sys.argv[0]).name} UPSTREAM_REPO", file=sys.stderr)
        return 2
    root = Path(__file__).resolve().parent.parent
    upstream = Path(sys.argv[1]).resolve()
    try:
        lock = load_json(root / "upstream.lock")
        snapshot = load_json(root / "internal/contract/official-routes-2.8.0.json")
        official = lock["officialContract"]
        if not isinstance(official, dict):
            return fail("upstream.lock officialContract must be an object")
        for key in ("repository", "tag", "tagObject", "commit", "packageVersion"):
            if official.get(key) != snapshot.get(key):
                return fail(f"reviewed snapshot {key} does not match upstream.lock")
        actual_commit = output("git", "-C", str(upstream), "rev-parse", "HEAD")
        if actual_commit != official["commit"]:
            return fail(
                f"checkout commit {actual_commit} is not reviewed commit {official['commit']}"
            )
        package = load_json(upstream / "package.json")
        if package.get("name") != "@remnawave/node":
            return fail("upstream package name is not @remnawave/node")
        if package.get("version") != official["packageVersion"]:
            return fail(
                f"package version {package.get('version')} does not match "
                f"{official['packageVersion']}"
            )
        local_version = (root / "internal/version/contract.version").read_text().strip()
        if local_version != official["packageVersion"]:
            return fail(
                f"local contract.version {local_version} does not match reviewed "
                f"{official['packageVersion']}"
            )
        extracted = output(
            sys.executable,
            str(root / "scripts/extract-contract-routes.py"),
            str(upstream),
        ).splitlines()
        routes = snapshot.get("routes")
        if not isinstance(routes, list) or not all(isinstance(route, str) for route in routes):
            return fail("reviewed route snapshot must contain a string array")
        if extracted != routes:
            missing = sorted(set(extracted) - set(routes))
            removed = sorted(set(routes) - set(extracted))
            return fail(f"official route drift: added={missing}, removed={removed}")
    except (KeyError, OSError, ValueError, subprocess.CalledProcessError) as error:
        return fail(str(error))
    print(
        f"Verified @remnawave/node {official['packageVersion']} at "
        f"{official['commit']} with {len(routes)} official routes."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
