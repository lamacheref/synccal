#!/usr/bin/env python3
"""
Auto-versioning script for SyncCal (M.m.f)

Rules:
- feat: commits bump minor (m), reset fix (f) to 0
- fix:/chore:/docs:/refactor:/test:/ci:/build: commits bump fix (f)
- Major (M) only bumped manually by user via tag
"""

import re
import subprocess
import sys
from pathlib import Path


def get_latest_tag() -> str:
    """Get the latest semver tag."""
    try:
        result = subprocess.run(
            ["git", "describe", "--tags", "--abbrev=0", "--match=v*"],
            capture_output=True,
            text=True,
            check=True,
        )
        return result.stdout.strip()
    except subprocess.CalledProcessError:
        return "v0.0.0"


def parse_version(tag: str) -> tuple[int, int, int]:
    """Parse version tag into (major, minor, fix)."""
    match = re.match(r"v?(\d+)\.(\d+)\.(\d+)", tag)
    if not match:
        return (0, 0, 0)
    return tuple(map(int, match.groups()))


def get_commits_since_tag(tag: str) -> list[str]:
    """Get commit messages since the given tag."""
    if tag == "v0.0.0":
        result = subprocess.run(
            ["git", "log", "--oneline", "--pretty=format:%s"],
            capture_output=True,
            text=True,
            check=True,
        )
    else:
        result = subprocess.run(
            ["git", "log", f"{tag}..HEAD", "--oneline", "--pretty=format:%s"],
            capture_output=True,
            text=True,
            check=True,
        )
    return [line for line in result.stdout.strip().split("\n") if line]


def classify_commit(msg: str) -> str:
    """Classify commit type from message."""
    msg_lower = msg.lower()
    if msg_lower.startswith("feat:"):
        return "feat"
    if msg_lower.startswith("fix:"):
        return "fix"
    for prefix in ("chore:", "docs:", "refactor:", "test:", "ci:", "build:", "perf:", "style:"):
        if msg_lower.startswith(prefix):
            return "other"
    return "other"


def compute_next_version(current: tuple[int, int, int], commits: list[str]) -> tuple[int, int, int]:
    """Compute next version based on commits since last tag."""
    major, minor, fix = current
    has_feat = False
    has_other = False

    for msg in commits:
        ctype = classify_commit(msg)
        if ctype == "feat":
            has_feat = True
        elif ctype in ("fix", "other"):
            has_other = True

    if has_feat:
        minor += 1
        fix = 0
    elif has_other:
        fix += 1
    # Major never auto-bumped

    return (major, minor, fix)


def main():
    tag = get_latest_tag()
    current = parse_version(tag)
    commits = get_commits_since_tag(tag)
    next_ver = compute_next_version(current, commits)

    print(f"Current: {current[0]}.{current[1]}.{current[2]}")
    print(f"Commits since {tag}: {len(commits)}")
    for c in commits:
        print(f"  - {c}")
    print(f"Next:    {next_ver[0]}.{next_ver[1]}.{next_ver[2]}")

    # Output for CI consumption
    print(f"::set-output name=version::{next_ver[0]}.{next_ver[1]}.{next_ver[2]}")
    print(f"::set-output name=tag::v{next_ver[0]}.{next_ver[1]}.{next_ver[2]}")


if __name__ == "__main__":
    main()