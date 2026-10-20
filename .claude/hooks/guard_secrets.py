#!/usr/bin/env python3
from __future__ import annotations

import fnmatch
import json
import os
import re
import shlex
import sys

ALLOWED_NAMES = {".env.example"}
SECRET_SUFFIXES = (".pem", ".key", ".p12", ".pfx", ".keystore")
SECRET_NAMES = {"id_rsa", "id_ecdsa", "id_ed25519", "id_dsa", ".envrc"}
SECRET_DIRECTORIES = ("/.ssh/", "/.aws/", "/.gnupg/")
GLOB_PROBES = (".env", ".env.local", ".env.production", ".envrc", "id_rsa", "server.key", "cert.pem", "bundle.p12")
FILE_TOOLS = {"Read", "Edit", "Write", "MultiEdit", "NotebookEdit"}
SEARCH_TOOLS = {"Grep", "Glob"}
INTERPRETERS = {"python", "python3", "node", "ruby", "perl", "bash", "sh", "zsh", "uv", "deno"}
FILE_ACCESS_HINTS = ("open(", "path(", "read", "cat ", "load", "subprocess", "exec", "system(")
SAFE_GIT_SUBCOMMANDS = {"check-ignore", "commit", "log", "status"}
TOKEN_SEPARATORS = re.compile(r"[\s;&|<>()`$=,\[\]{}]+")
HEREDOC_START = re.compile(r"<<-?\s*(['\"]?)([^\s'\"<>;|&()]+)\1")
GLOB_CHARACTERS = re.compile(r"[*?\[]")


def normalised_name(path: str) -> str:
    return os.path.basename(path.strip().strip("'\"")).lower()


def is_secret_name(name: str) -> bool:
    if not name or name in ALLOWED_NAMES or name.endswith(".example"):
        return False
    if name == ".env" or name.startswith(".env.") or name.endswith(".env"):
        return True
    return name in SECRET_NAMES or name.endswith(SECRET_SUFFIXES)


def is_secret_path(path: str) -> bool:
    cleaned = path.strip().strip("'\"")
    if not cleaned:
        return False
    lowered = cleaned.lower()
    if any(directory in lowered for directory in SECRET_DIRECTORIES) or lowered.startswith(("~/.ssh", "~/.aws", "~/.gnupg")):
        return True
    name = normalised_name(cleaned)
    if GLOB_CHARACTERS.search(name):
        return any(fnmatch.fnmatch(probe, name) for probe in GLOB_PROBES)
    return is_secret_name(name)


def resolved_is_secret(path: str) -> bool:
    if is_secret_path(path):
        return True
    expanded = os.path.expanduser(path)
    return os.path.islink(expanded) and is_secret_path(os.path.realpath(expanded))


def split_heredocs(command: str) -> tuple[str, list[tuple[str, bool, str]]]:
    kept: list[str] = []
    bodies: list[tuple[str, bool, str]] = []
    pending: list[tuple[str, bool, str]] = []
    body: list[str] = []
    for line in command.splitlines():
        if pending:
            delimiter, quoted, opener = pending[0]
            if line.strip() == delimiter:
                bodies.append(("\n".join(body), quoted, opener))
                body = []
                pending.pop(0)
            else:
                body.append(line)
            continue
        kept.append(line)
        for match in HEREDOC_START.finditer(line):
            pending.append((match.group(2), bool(match.group(1)), line))
    if pending:
        bodies.append(("\n".join(body), False, pending[0][2]))
    return "\n".join(kept), bodies


def command_tokens(command: str) -> list[str]:
    try:
        words = shlex.split(command, posix=True)
    except ValueError:
        words = command.split()
    tokens: list[str] = []
    for word in words:
        tokens.extend(part for part in TOKEN_SEPARATORS.split(word) if part)
    return tokens


def is_safe_git(segment: str) -> bool:
    try:
        words = shlex.split(segment, posix=True)
    except ValueError:
        return False
    if len(words) < 2 or os.path.basename(words[0]) != "git":
        return False
    rest = [word for word in words[1:] if not word.startswith("-")]
    return bool(rest) and rest[0] in SAFE_GIT_SUBCOMMANDS


def shell_secret(command: str) -> str | None:
    for segment in re.split(r"&&|\|\||;|\n", command):
        if is_safe_git(segment.strip()):
            continue
        for token in command_tokens(segment):
            if is_secret_path(token):
                return token
    return None


def opener_is_interpreter(opener: str) -> bool:
    try:
        words = shlex.split(opener.split("<<", 1)[0], posix=True)
    except ValueError:
        words = opener.split()
    return any(os.path.basename(word) in INTERPRETERS or os.path.basename(word).startswith("python") for word in words)


def heredoc_secret(body: str, quoted: bool, opener: str) -> str | None:
    if not quoted:
        return shell_secret(body)
    if not opener_is_interpreter(opener):
        return None
    for line in body.splitlines():
        lowered = line.lower()
        if any(hint in lowered for hint in FILE_ACCESS_HINTS):
            for token in command_tokens(line):
                if is_secret_path(token):
                    return token
    return None


def blocked_target(payload: dict[str, object]) -> str | None:
    tool = payload.get("tool_name")
    tool_input = payload.get("tool_input")
    if not isinstance(tool, str) or not isinstance(tool_input, dict):
        return "malformed hook payload"
    if tool in FILE_TOOLS:
        for key in ("file_path", "notebook_path"):
            value = tool_input.get(key)
            if isinstance(value, str) and resolved_is_secret(value):
                return value
        return None
    if tool in SEARCH_TOOLS:
        for key in ("path", "pattern", "glob"):
            value = tool_input.get(key)
            if isinstance(value, str) and tool == "Glob" and key == "pattern" and is_secret_path(value):
                return value
            if isinstance(value, str) and key != "pattern" and is_secret_path(value):
                return value
        return None
    if tool == "Bash":
        command = tool_input.get("command")
        if not isinstance(command, str):
            return None
        outer, bodies = split_heredocs(command)
        found = shell_secret(outer)
        if found:
            return found
        for body, quoted, opener in bodies:
            found = heredoc_secret(body, quoted, opener)
            if found:
                return found
    return None


def main() -> int:
    try:
        payload = json.load(sys.stdin)
        target = blocked_target(payload) if isinstance(payload, dict) else "malformed hook payload"
    except Exception as error:
        target = f"hook failure ({type(error).__name__})"
    if target is None:
        return 0
    print(
        f"Blocked: {target} looks like a secret (environment file, key or credential directory), "
        "or the guard could not check the call. Agents never read or edit secrets in this repository; ask the owner instead.",
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    sys.exit(main())
