#!/usr/bin/env python3
from __future__ import annotations

import json
import subprocess
import sys


def edited_path(payload: dict[str, object]) -> str:
    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        return ""
    path = tool_input.get("file_path")
    return path if isinstance(path, str) else ""


def compile_error(path: str) -> str | None:
    result = subprocess.run([sys.executable, "-m", "py_compile", path], capture_output=True, text=True)
    return result.stderr.strip() if result.returncode != 0 else None


def main() -> int:
    payload = json.load(sys.stdin)
    path = edited_path(payload)
    if not path.endswith(".py"):
        return 0
    error = compile_error(path)
    if error is None:
        return 0
    print(f"py_compile failed for {path}:\n{error}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
