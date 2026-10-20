#!/usr/bin/env python3
from __future__ import annotations

import json
import shutil
import subprocess
import sys


def edited_path(payload: dict[str, object]) -> str:
    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict):
        return ""
    path = tool_input.get("file_path")
    return path if isinstance(path, str) else ""


def python_error(path: str) -> str | None:
    result = subprocess.run([sys.executable, "-m", "py_compile", path], capture_output=True, text=True)
    return f"py_compile failed for {path}:\n{result.stderr.strip()}" if result.returncode != 0 else None


def go_error(path: str) -> str | None:
    gofmt = shutil.which("gofmt")
    if gofmt is None:
        return f"gofmt is not installed, so {path} was not checked; install Go from the development guide"
    result = subprocess.run([gofmt, "-l", "--", path], capture_output=True, text=True)
    if result.returncode != 0:
        return f"gofmt could not parse {path}:\n{result.stderr.strip()}"
    if result.stdout.strip():
        return f"{path} is not gofmt-formatted; run gofmt -w on it"
    return None


def main() -> int:
    payload = json.load(sys.stdin)
    path = edited_path(payload)
    if path.endswith(".py"):
        error = python_error(path)
    elif path.endswith(".go"):
        error = go_error(path)
    else:
        return 0
    if error is None:
        return 0
    print(error, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
