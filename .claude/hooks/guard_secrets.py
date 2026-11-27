#!/usr/bin/env python3
from __future__ import annotations

import fnmatch
import hashlib
import json
import os
import pwd
import re
import shlex
import stat
import subprocess
import sys
import tempfile

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
HEREDOC_START = re.compile(r"(?<![<$(])<<(?!<)-?\s*(['\"]?)([^\s'\"<>;|&()]+)\1")
ARITHMETIC = re.compile(r"\$\(\([^)]*\)\)")
GLOB_CHARACTERS = re.compile(r"[*?\[]")
SECRET_VARIABLE = re.compile(r"\$\{?[A-Za-z_]*(PASSWORD|SECRET|TOKEN|ACCESS_KEY|SECRET_KEY|DATABASE_URL|ADMIN_URL)[A-Za-z_]*", re.IGNORECASE)


def normalised_name(path: str) -> str:
    return os.path.basename(path.strip().strip("'\"")).lower()


def is_secret_name(name: str) -> bool:
    if not name or name in ALLOWED_NAMES or name.endswith(".example"):
        return False
    if name in {"process.env", "import.meta.env", "bun.env", "deno.env"}:
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
        for match in HEREDOC_START.finditer(ARITHMETIC.sub("", line)):
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


SHELL_PUNCTUATION = ";&|()<>\n"
SHELLS = {"sh", "bash", "zsh", "dash", "ksh"}
WRAPPERS = {
    "command", "builtin", "exec", "nohup", "time", "sudo", "xargs", "stdbuf", "caffeinate", "watch", "arch", "dev-env.sh",
    ".", "source",
    "{", "!", "if", "then", "else", "elif", "do", "while", "until",
}
WRAPPER_OPTIONS_WITH_VALUES = {
    "nice": {"-n"},
    "exec": {"-a"},
    "timeout": {"-s", "-k", "--signal", "--kill-after"},
    "env": {"-u", "--unset", "-C", "--chdir", "-S", "--split-string"},
    "watch": {"-n", "-d", "--interval"},
    "xargs": {"-I", "-n", "-L", "-P", "-s", "-E", "-J", "-R"},
    "uv": {"--with", "--python", "-p", "--project", "--directory", "--env-file", "--package"},
    "sudo": {"-u", "-g", "-C", "-h", "-p"},
    "pnpm": {"--dir", "-C", "--filter", "-F"},
    "pnpm-dlx": {"--package"},
    "npx": {"-p", "--package", "-c", "--call"},
    "pnpx": {"-p", "--package", "-c", "--call"},
}
PACKAGE_RUNNERS = {"npx", "pnpx"}
PNPM_RUNNERS = {"exec", "dlx"}
RUNNER_PROGRAMS = {"pnpm", "npm", "npx", "pnpx"}
RUNNER_SCRIPT_OPTIONS = {"-c", "--call", "--shell-mode"}
CONTAINER_TOOLS = {"docker", "docker-compose", "podman", "nerdctl"}
CONFIG_SUBCOMMANDS = {"config", "convert"}
READ_ONLY_CONFIG_FLAGS = {"--services", "--images", "--volumes", "--profiles", "--networks", "-q", "--quiet"}
ENVIRONMENT_CODE = re.compile(
    r"\bos\.environ\b|\bprocess\.env\b|\bENVIRON\b|%ENV\b|\bENV\[|\bENV\.(to_h|each|keys|inspect)|"
    r"\bDeno\.env\b|\bgetenv\s*\(|\bos\.Environ\b|\bSystem\.getenv\b|\$ENV\{|\bENV\.fetch\b|\benviron\b|"
    r"\bBun\.env\b|\$_ENV\b|\bimport\.meta\.env\b"
)
INTERPRETER_PROGRAMS = {"python", "python3", "node", "ruby", "perl", "awk", "gawk", "deno", "bun", "php", "lua"}
VALUE_PRINTERS = {"echo", "printf", "print", "cat", "tee"}
PS_OPTIONS_WITH_VALUES = {"-o", "-O", "-p", "-U", "-u", "-t", "-G", "-g"}
SUBSTITUTION = re.compile(r"\$\(([^()]*)\)|`([^`]*)`")
MAX_NESTING = 4


def shell_words(command: str) -> list[str]:
    lexer = shlex.shlex(command, posix=True, punctuation_chars=SHELL_PUNCTUATION)
    lexer.whitespace = " \t\r"
    lexer.whitespace_split = True
    try:
        return list(lexer)
    except ValueError:
        return command.split()


def simple_commands(command: str) -> list[list[str]]:
    commands: list[list[str]] = [[]]
    for word in shell_words(command):
        if word and all(character in SHELL_PUNCTUATION for character in word):
            commands.append([])
        else:
            commands[-1].append(word)
    return [words for words in commands if words]


def is_assignment(word: str) -> bool:
    name, separator, _ = word.partition("=")
    return bool(separator) and bool(name) and name.replace("_", "a").isalnum() and not name[0].isdigit()


def skip_options(program: str, rest: list[str]) -> list[str]:
    with_values = WRAPPER_OPTIONS_WITH_VALUES.get(program, set())
    while rest and (rest[0].startswith("-") or (program == "env" and is_assignment(rest[0]))):
        option = rest[0]
        takes_value = option in with_values or (
            not option.startswith("--") and len(option) > 2 and any(f"-{letter}" in with_values for letter in option[-1])
        )
        rest = rest[2:] if takes_value else rest[1:]
    return rest


def with_package_program(words: list[str]) -> list[str]:
    return [package_program(words[0]), *words[1:]] if words else words


def unwrap(words: list[str]) -> list[str]:
    while words:
        if is_assignment(words[0]):
            words = words[1:]
            continue
        program = os.path.basename(words[0])
        if program == "env":
            rest = skip_options(program, words[1:])
            if not rest:
                return ["env"]
            words = rest
            continue
        if program == "timeout":
            rest = skip_options(program, words[1:])
            words = rest[1:]
            continue
        if program == "uv" and len(words) > 1 and words[1] == "run":
            words = skip_options(program, words[2:])
            continue
        if program in PACKAGE_RUNNERS:
            words = with_package_program(skip_options(program, words[1:]))
            continue
        if program == "pnpm":
            rest = skip_options(program, words[1:])
            if rest and rest[0] in PNPM_RUNNERS:
                words = with_package_program(skip_options(f"pnpm-{rest[0]}", rest[1:]))
                continue
            return words
        if program == "command" and len(words) > 1 and words[1] in {"-v", "-V"}:
            return words
        if program == "nice" or program in WRAPPERS:
            words = skip_options(program, words[1:])
            continue
        return words
    return words


def shell_command_argument(arguments: list[str]) -> str | None:
    for index, word in enumerate(arguments):
        if word.startswith("-") and not word.startswith("--") and "c" in word[1:]:
            rest = arguments[index + 1 :]
            if rest and rest[0] == "--":
                rest = rest[1:]
            return rest[0] if rest else None
    return None


def ps_prints_environments(arguments: list[str]) -> bool:
    skip = False
    for index, word in enumerate(arguments):
        if skip:
            skip = False
            continue
        if word in PS_OPTIONS_WITH_VALUES:
            skip = True
            continue
        if word.startswith("-") and not word.startswith("--") and "E" in word:
            return True
        if not word.startswith("-") and word.isalpha() and word.islower() and "e" in word and len(word) <= 6:
            return True
    return False


def container_printer(arguments: list[str], depth: int) -> str | None:
    positional = [word for word in arguments if not word.startswith("-")]
    if "inspect" in positional:
        inspected = positional[: positional.index("inspect")]
        if not {"image", "volume", "network"} & set(inspected):
            return "a container command that prints interpolated credentials"
    config = next((word for word in arguments if word in CONFIG_SUBCOMMANDS), None)
    if config is not None:
        flags = {word for word in arguments[arguments.index(config) + 1 :] if word.startswith("-")}
        if not (flags and flags <= READ_ONLY_CONFIG_FLAGS):
            return "a container command that prints interpolated credentials"
    flags = {word for word in arguments if word.startswith("-")}
    if "bake" in positional and "--print" in flags:
        return "a container command that prints interpolated credentials"
    if "exec" in positional or "run" in positional:
        anchor = arguments.index("exec") if "exec" in arguments else arguments.index("run")
        tail = arguments[anchor + 1 :]
        for offset in range(len(tail)):
            found = words_printer(tail[offset:], depth + 1)
            if found:
                return found
        for word in tail:
            if " " in word or "|" in word or ";" in word:
                found = command_printer(word, depth + 1)
                if found:
                    return found
    return None


SETTINGS_WRAPPER = "dev-env.sh"
TRACE_VARIABLES = ("SHELLOPTS=", "BASHOPTS=", "BASH_ENV=", "BASH_XTRACEFD=", "ENV=", "ZDOTDIR=", "HOME=")


SHELL_OPTIONS_WITH_VALUES = {"-o", "+o", "-O", "+O", "--rcfile", "--init-file"}
TRACE_OPTION_NAMES = {"xtrace", "verbose"}


def takes_option_value(word: str) -> bool:
    return word in SHELL_OPTIONS_WITH_VALUES or (word[:1] in {"-", "+"} and not word.startswith("--") and len(word) > 2 and word[-1] in {"o", "O"})


def shell_options(arguments: list[str]) -> tuple[list[str], int]:
    options: list[str] = []
    index = 0
    while index < len(arguments):
        word = arguments[index]
        if word == "--":
            return options, index + 1
        if takes_option_value(word):
            options.extend(arguments[index : index + 2])
            index += 2
            continue
        if word.startswith(("-", "+")) and len(word) > 1:
            options.append(word)
            index += 1
            continue
        break
    return options, index


def shell_traces(arguments: list[str]) -> bool:
    options, _ = shell_options(arguments)
    for index, word in enumerate(options):
        if not word.startswith("-") or word.startswith("--"):
            continue
        if takes_option_value(word) and index + 1 < len(options) and options[index + 1].lower() in TRACE_OPTION_NAMES:
            return True
        if word not in SHELL_OPTIONS_WITH_VALUES and ("x" in word[1:] or "v" in word[1:]):
            return True
    return False


def script_turns_on_tracing(script: str) -> bool:
    for words in simple_commands(script):
        command = unwrap(words)
        if not command:
            continue
        program = os.path.basename(command[0])
        if program == "set" and shell_traces(command[1:]):
            return True
        if program in {"shopt", "setopt"} and any(word.lower() in TRACE_OPTION_NAMES for word in command[1:]):
            return True
    return False


def shell_script(arguments: list[str]) -> tuple[str, list[str]] | None:
    options, index = shell_options(arguments)
    if any(word.startswith("-") and not word.startswith("--") and not takes_option_value(word) and "c" in word[1:] for word in options):
        return None
    if index >= len(arguments):
        return None
    return arguments[index], arguments[index + 1 :]


def settings_wrapper_trace(words: list[str]) -> str | None:
    names = [os.path.basename(word) for word in words]
    if SETTINGS_WRAPPER not in names:
        return None
    position = names.index(SETTINGS_WRAPPER)
    if any(word.startswith(TRACE_VARIABLES) for word in words):
        return "a shell trace setting that would print the loaded settings"
    for index, name in enumerate(names[:position]):
        if name in SHELLS and shell_traces(words[index + 1 : position]):
            return "a traced shell that would print the loaded settings"
    inner = unwrap(words[position + 1 :])
    if inner and os.path.basename(inner[0]) in SHELLS:
        if shell_traces(inner[1:]):
            return "a traced shell that would print the loaded settings"
        script = shell_command_argument(inner[1:])
        if script is not None and script_turns_on_tracing(script):
            return "a traced shell that would print the loaded settings"
    return None


def runner_printer(arguments: list[str], depth: int) -> str | None:
    for index, word in enumerate(arguments):
        name, separator, value = word.partition("=")
        if name not in RUNNER_SCRIPT_OPTIONS:
            continue
        script = value if separator else (arguments[index + 1] if index + 1 < len(arguments) else None)
        if script is not None:
            found = command_printer(script, depth + 1)
            if found:
                return found
    for offset in range(len(arguments)):
        found = words_printer(arguments[offset:], depth + 1)
        if found:
            return found
    return None


def words_printer(words: list[str], depth: int) -> str | None:
    if depth > MAX_NESTING:
        return "a command nested too deeply to check"
    traced = settings_wrapper_trace(words)
    if traced:
        return traced
    for index, word in enumerate(words):
        if os.path.basename(word) in RUNNER_PROGRAMS:
            found = runner_printer(words[index + 1 :], depth)
            if found:
                return found
            break
    words = unwrap(words)
    if not words:
        return None
    if is_safe_git(" ".join(shlex.quote(word) for word in words)):
        return None
    if any("/proc/" in word and "environ" in word for word in words):
        return "a read of a process environment"
    program = os.path.basename(words[0])
    arguments = words[1:]
    if program in {"command", "type", "which", "whence"} and arguments and arguments[0] in {"-v", "-V", "-a"}:
        return None
    if program == "printenv" or (program == "env" and not arguments):
        return "a command that prints the environment"
    if program in {"export", "declare", "typeset", "compgen"} and all(word.startswith("-") for word in arguments):
        return "a command that prints the environment"
    if program == "set" and not arguments:
        return "a command that prints the environment"
    if program == "ps" and ps_prints_environments(arguments):
        return "a command that prints process environments"
    if program == "launchctl" and ("getenv" in arguments or "export" in arguments):
        return "a command that prints the environment"
    if program == "eval":
        return command_printer(" ".join(arguments), depth + 1)
    if program in SHELLS:
        script = shell_script(arguments)
        if script is not None and os.path.basename(script[0]) == SETTINGS_WRAPPER:
            return words_printer(script[1], depth + 1)
        inner = shell_command_argument(arguments)
        if inner is not None:
            return command_printer(inner, depth + 1)
    if program in CONTAINER_TOOLS:
        return container_printer(arguments, depth)
    if program.startswith("python") or program in INTERPRETER_PROGRAMS:
        if any(ENVIRONMENT_CODE.search(word) for word in arguments):
            return "an interpreter command that reads the environment"
    if program in VALUE_PRINTERS and any(SECRET_VARIABLE.search(word) for word in arguments):
        return "a command that prints a credential variable"
    return None


def without_single_quoted(text: str) -> str:
    kept: list[str] = []
    quote: str | None = None
    for character in text:
        if quote is None and character in {"'", '"'}:
            quote = character
        elif quote is not None and character == quote:
            quote = None
            continue
        if quote != "'" and not (quote is None and character == "'"):
            kept.append(character)
    return "".join(kept)


def substitution_printer(text: str, depth: int) -> str | None:
    for match in SUBSTITUTION.finditer(without_single_quoted(text)):
        inner = match.group(1) if match.group(1) is not None else match.group(2)
        found = command_printer(inner, depth + 1)
        if found:
            return found
    return None


def command_printer(command: str, depth: int = 0) -> str | None:
    if depth > MAX_NESTING:
        return "a command nested too deeply to check"
    if re.search(r"<<<\s*\S*" + SECRET_VARIABLE.pattern, command, re.IGNORECASE):
        return "a command that prints a credential variable"
    found = substitution_printer(command, depth)
    if found:
        return found
    for words in simple_commands(command):
        found = words_printer(words, depth)
        if found:
            return found
    return None


def shell_secret(command: str, check_printers: bool = True) -> str | None:
    if check_printers:
        printer = command_printer(command)
        if printer:
            return printer
    for segment in re.split(r"&&|\|\||;|\n", command):
        if is_safe_git(segment.strip()):
            continue
        for token in command_tokens(segment):
            if is_secret_path(token):
                return token
    return None


def opener_words(opener: str) -> list[str]:
    try:
        words = shlex.split(opener.split("<<", 1)[0], posix=True)
    except ValueError:
        words = opener.split()
    return [os.path.basename(word) for word in words]


def opener_is_interpreter(opener: str) -> bool:
    return any(word in INTERPRETERS or word.startswith("python") for word in opener_words(opener))


def opener_is_shell(opener: str) -> bool:
    return any(word in SHELLS for word in opener_words(opener))


def heredoc_secret(body: str, quoted: bool, opener: str) -> str | None:
    if opener_is_shell(opener):
        return shell_secret(body)
    if opener_is_interpreter(opener) and ENVIRONMENT_CODE.search(body):
        return "an interpreter command that reads the environment"
    if not quoted:
        if SECRET_VARIABLE.search(body):
            return "a command that prints a credential variable"
        found = substitution_printer(body, 0)
        if found:
            return found
        return shell_secret(body, check_printers=False)
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
            return "malformed hook payload"
        outer, bodies = split_heredocs(command)
        found = shell_secret(outer)
        if found:
            return found
        for body, quoted, opener in bodies:
            found = heredoc_secret(body, quoted, opener)
            if found:
                return found
    return None


BRANCH_NAME = re.compile(r"^(feat|fix|chore|docs)/[a-z0-9-]+$")
FULL_SHA = re.compile(r"^[0-9a-f]{40}$")
API_PATH = re.compile(
    r"^repos/klukashka/ovc/(actions/(runs|jobs)(/(?![.]+(?:/|[?]|$))[A-Za-z0-9_.-]+)*|commits/[0-9a-f]{40}/(status|statuses))"
    r"(\?[A-Za-z0-9_.,:=&%-]*)?$"
)
PR_SELECTOR = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]*$")
ALIAS_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9-]*$")
ACCEPT_HEADER = re.compile(r"^accept\s*:", re.IGNORECASE)
GIT_GLOBAL_OPTIONS_WITH_VALUES = {
    "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path", "--config-env", "--super-prefix", "--attr-source",
}
GIT_CONFIG_OVERRIDES = {"-c", "--config-env"}
PUSH_FAMILY_SUBCOMMANDS = {"push", "send-pack", "remote", "config"}
PUSH_FAMILY_PROGRAMS = {"git-push", "git-send-pack", "git-remote", "git-config"}
GIT_BUILTINS = {
    "add", "am", "annotate", "apply", "archive", "bisect", "blame", "branch", "bundle", "cat-file", "check-attr",
    "check-ignore", "check-mailmap", "check-ref-format", "checkout", "checkout-index", "cherry", "cherry-pick", "clean",
    "clone", "column", "commit", "commit-graph", "commit-tree", "count-objects", "credential", "describe", "diff",
    "diff-files", "diff-index", "diff-tree", "difftool", "fetch", "for-each-ref", "format-patch", "fsck", "gc", "grep",
    "hash-object", "help", "init", "interpret-trailers", "lfs", "log", "ls-files", "ls-remote", "ls-tree", "maintenance",
    "merge", "merge-base", "merge-file", "merge-tree", "mergetool", "mktree", "mv", "name-rev", "notes", "pull",
    "range-diff", "read-tree", "rebase", "reflog", "repack", "replace", "rerere", "reset", "restore", "rev-list",
    "rev-parse", "revert", "rm", "shortlog", "show", "show-branch", "show-ref", "sparse-checkout", "stash", "status",
    "submodule", "switch", "symbolic-ref", "tag", "update-index", "update-ref", "var", "verify-commit", "verify-tag",
    "version", "whatchanged", "worktree", "write-tree",
}
GH_AUTH_DENIED = {"token", "git-credential", "login", "refresh", "switch"}
GH_TOKEN_SETTING = "oauth_token"
OPAQUE_PROGRAMS = INTERPRETER_PROGRAMS | {"eval", "find", "fd", "parallel", "trap", "osascript", "expect", "ssh"}
SPLIT_STRING_OPTIONS = ("-S", "--split-string")
DYNAMIC_CHARACTERS = ("$", "`")
CREDENTIAL_ACTIONS = {"fill", "approve", "reject"}
KEYCHAIN_HELPERS = {"credential-osxkeychain", "git-credential-osxkeychain"}
KEYCHAIN_QUERIES = {"find-internet-password", "find-generic-password"}
GH_CONFIG_DIRECTORY = ".config/gh"
GH_HOSTS_FILE = ".config/gh/hosts.yml"
GH_REPOSITORY_OPTIONS = ("-R", "--repo", "--hostname")
GH_RUN_READS = {"view", "list", "watch"}
GATE_SCOPES = {"full", "docs"}
PR_FIELDS = {"number", "state", "headRefName", "headRefOid", "url"}
PR_OPTIONS_WITH_VALUES = {
    "view": {"--json", "--jq", "-q", "--template", "-t"},
    "list": {"--json", "--jq", "-q", "--template", "-t", "--state", "-s", "--head", "-H", "--base", "-B", "--limit", "-L"},
}
API_OPTIONS_WITH_VALUES = {"--jq", "-q", "--template", "-t", "--cache"}
API_FLAGS = {"--paginate", "--slurp", "--include", "-i", "--silent", "--verbose"}
CONFIG_READ_FLAGS = {"--get", "--get-all", "--get-regexp", "--get-urlmatch", "--list", "-l", "--get-color", "--get-colorbool"}
CONFIG_NEUTRAL_FLAGS = {
    "--global", "--system", "--local", "--worktree", "--show-origin", "--show-scope", "--null", "-z", "--name-only",
    "--includes", "--no-includes", "--bool", "--int", "--path", "--bool-or-int", "--expiry-date", "--all", "--regexp",
    "--show-names",
}
CONFIG_OPTIONS_WITH_VALUES = {"--file", "-f", "--blob", "--type", "--default"}
CONFIG_READ_SUBCOMMANDS = {"get", "list"}
CONFIG_WRITE_SUBCOMMANDS = {"set", "unset", "rename-section", "remove-section", "edit"}
REMOTE_GET_URL_FLAGS = {"--push", "--all"}
UNSAFE_CHARACTERS = ("$", "`", "\\", "\n")
GIT_TIMEOUT_SECONDS = 10
SCANNER_TIMEOUT_SECONDS = 45
LOCK_DIR_OPTION = "--lock-dir"
LOCK_NAME = "merge-gate.lock"
LOG_NAME = "merge-gate.log"
LOCK_KEY = re.compile(r"^[a-z_]+$")
LOCK_STATES = {"running", "passed", "dispatched", "evidence", "fallback"}
GATE_KINDS = {"gate", "after-fix"}
CATCH_UP_KIND = "catch-up"
CI_RECORDS = {"ci", "ci-catch-up"}
FINISH_PREFIX = "finish: "
FINISH_KEYS = {"record", "head", "tree", "result"}
FINISH_PASSED = "passed"
CATCH_UP_BRANCH = re.compile(r"^catch-up/[0-9a-f]{12}$")
REBUILD_SUFFIX = re.compile(r"^-r[1-9][0-9]*$")
EVIDENCE_ENDINGS = ("-before", "-after")
EVIDENCE_BRANCH_SUFFIXES = {"before-first": "-before", "before-rest": "-before", "after": "-after"}
BEFORE_SERIES = {"before-first", "before-rest"}
DISPATCH_INPUT_NAME = re.compile(r"^[a-z]+$")
CHANGE_SCOPE = "change"
EVIDENCE_SCOPE = "evidence"
CI_JOB_SHARDS = {"backend-tests": 3, "backend-checks": 0, "frontend-checks": 0, "e2e": 12, "docs-checks": 0}
SHARD_NUMBER = re.compile(r"^[1-9][0-9]?$")
EVIDENCE_SELECTIONS = {"go": ("package", "test"), "playwright": ("project", "title"), "check": ("entry",)}
EVIDENCE_VALUES = {
    "package": re.compile(r"^[A-Za-z0-9_./][A-Za-z0-9_./-]{0,199}$"),
    "test": re.compile(r"^Test(?!.*//)(?!.*/$)[A-Za-z0-9_/]{1,196}$"),
    "project": re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$"),
    "title": re.compile(r"^[A-Za-z0-9](?:[A-Za-z0-9 ,:._()/@+?›’×–…»«“”-]{0,198}[A-Za-z0-9,:._()/@+?›’×–…»«“”-])?$"),
    "entry": re.compile(r"^(backend-checks|frontend-checks|docs-checks):[1-9][0-9]{0,2}$"),
}
EMAIL_ADDRESS = re.compile(r"(?<![A-Za-z0-9._%+-])[A-Za-z0-9._%+-]+@((?:[A-Za-z0-9-]+\.)+[A-Za-z]{2,})(?![A-Za-z0-9-])")
RUSSIAN_MOBILE = re.compile(
    r"(?<![0-9A-Za-z])(?:\+7|8)[ \t(-]*9[0-9]{2}[ \t)-]*[0-9]{3}[ \t-]*[0-9]{2}[ \t-]*[0-9]{2}(?![0-9A-Za-z])"
)
RESERVED_TOP_LEVEL_DOMAINS = {"test", "invalid", "localhost", "example"}
RESERVED_SECOND_LEVEL_LABEL = "example"
NOREPLY_DOMAIN = "users.noreply.github.com"
DIFF_HEADERS = (
    "diff --git ", "index ", "--- ", "+++ ", "@@", "new file mode", "deleted file mode", "similarity index", "rename from",
    "rename to", "old mode", "new mode", "Binary files",
)
CHECK_PROGRAMS = {"golangci-lint", "govulncheck", "gitleaks", "osv-scanner", "vitest", "eslint", "tsc"}
CHECK_TOOL_NAMES = ("golangci-lint", "govulncheck", "gitleaks", "osv-scanner")
GO_CHECK_SUBCOMMANDS = {"test", "vet"}
GO_TOOL_RUNNERS = {"run", "tool"}
GO_OPTIONS_WITH_VALUES = {"-C"}
GOFMT_CHECK_FLAGS = {"-l", "-d"}
SQLC_CHECK_SUBCOMMANDS = {"diff", "vet", "compile"}
SQLC_OPTIONS_WITH_VALUES = {"-f", "--file"}
PRETTIER_CHECK_FLAGS = {"--check", "-c", "--list-different", "-l"}
PNPM_CHECK_SCRIPTS = {"test", "t", "e2e", "lint", "typecheck", "format:check", "api:check", "audit"}
PNPM_CHECK_PREFIXES = ("check:", "test:", "e2e:", "lint:")
SCRIPT_RUNNERS = {"run", "run-script"}
NPM_EXEC_COMMANDS = {"exec", "x"}
PNPM_EXEC_COMMANDS = {"exec", "dlx"}
NPX_PROGRAMS = {"npx", "pnpx"}
PNPM_OPTIONS_WITH_VALUES = {"--dir", "-C", "--filter", "-F", "--reporter", "--loglevel", "--workspace-dir"}
NPM_OPTIONS_WITH_VALUES = {"--prefix", "-w", "--workspace", "--registry", "--loglevel", "-p", "--package"}
MERGE_GATE_SCRIPT = "merge-gate.sh"
MERGE_GATE_CHECK_SUBCOMMANDS = {"gate", "run-job"}
GUARD_CHECKER = "check_guard_secrets.py"
CHECK_SCRIPT_PATTERNS = ("check-*.sh", "*-test.sh")
MODULE_OPTION = "-m"


def flat_tokens(words: list[str]) -> list[str]:
    return [token.strip("'\"") for token in command_tokens(" ".join(shlex.quote(word) for word in words))]


def git_subcommand(tokens: list[str]) -> tuple[str | None, list[str], bool]:
    index = 0
    overrides_alias = False
    while index < len(tokens):
        token = tokens[index]
        if token in GIT_GLOBAL_OPTIONS_WITH_VALUES:
            following = tokens[index + 1] if index + 1 < len(tokens) else ""
            if token in GIT_CONFIG_OVERRIDES and following.lower().startswith("alias."):
                overrides_alias = True
            index += 2
            continue
        if token.startswith("-"):
            index += 1
            continue
        return token, tokens[index + 1 :], overrides_alias
    return None, [], overrides_alias


def touches_gh_credentials(token: str) -> bool:
    cleaned = token.rstrip("/")
    if GH_HOSTS_FILE in cleaned or cleaned.endswith(GH_CONFIG_DIRECTORY):
        return True
    return f"{GH_CONFIG_DIRECTORY}/" in cleaned and bool(GLOB_CHARACTERS.search(os.path.basename(cleaned)))


def gh_subcommand_rest(tokens: list[str], group: str, subcommand: str) -> list[str] | None:
    if group not in tokens:
        return None
    after_group = tokens[tokens.index(group) + 1 :]
    if subcommand not in after_group:
        return None
    return after_group[after_group.index(subcommand) + 1 :]


def is_short_flag_with(word: str, letter: str) -> bool:
    return word.startswith("-") and not word.startswith("--") and letter in word[1:]


def gh_auth_status_shows_token(tokens: list[str]) -> bool:
    rest = gh_subcommand_rest(tokens, "auth", "status")
    return rest is not None and any(is_short_flag_with(word, "t") for word in rest)


def gh_config_reads_token(tokens: list[str]) -> bool:
    rest = gh_subcommand_rest(tokens, "config", "get")
    return rest is not None and any(GH_TOKEN_SETTING in word for word in rest)


def denied_call(tokens: list[str]) -> str | None:
    if any(token == "--show-token" or token.startswith("--show-token=") for token in tokens):
        return "a command that prints a GitHub token"
    names = [os.path.basename(token) for token in tokens]
    for index, name in enumerate(names):
        rest = tokens[index + 1 :]
        if touches_gh_credentials(tokens[index]):
            return "a read of the GitHub CLI credential file"
        if name == "gh" and "auth" in rest and set(rest[rest.index("auth") + 1 :]) & GH_AUTH_DENIED:
            return "a GitHub CLI command that prints, stores or switches credentials"
        if name == "gh" and gh_auth_status_shows_token(rest):
            return "a command that prints a GitHub token"
        if name == "gh" and gh_config_reads_token(rest):
            return "a command that prints a GitHub token"
        if name in KEYCHAIN_HELPERS:
            return "a Git keychain credential helper call"
        if name == "git-credential" and set(rest) & CREDENTIAL_ACTIONS:
            return "a Git credential helper call"
        if name == "git":
            subcommand, after, _ = git_subcommand(rest)
            if subcommand in KEYCHAIN_HELPERS:
                return "a Git keychain credential helper call"
            if subcommand == "credential" and set(after) & CREDENTIAL_ACTIONS:
                return "a Git credential helper call"
        if name == "security" and set(rest) & KEYCHAIN_QUERIES:
            return "a keychain password query"
    return None


def child_environment() -> dict[str, str]:
    return {name: value for name, value in os.environ.items() if not name.startswith(("GIT_", "GH_"))}


def git_read(root: str, *arguments: str) -> str | None:
    try:
        result = subprocess.run(
            ["git", *arguments], cwd=root, capture_output=True, text=True, timeout=GIT_TIMEOUT_SECONDS,
            env=child_environment(), check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    return result.stdout.strip() if result.returncode == 0 else None


def git_alias_exists(name: str, cwd: str) -> bool:
    if not ALIAS_NAME.match(name):
        return False
    try:
        result = subprocess.run(
            ["git", "config", "--get", f"alias.{name}"], cwd=cwd, capture_output=True, text=True,
            timeout=GIT_TIMEOUT_SECONDS, env=child_environment(), check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return True
    return result.returncode != 1


def names_github_program(words: list[str]) -> bool:
    github_names = {"git", "gh"} | PUSH_FAMILY_PROGRAMS
    return any(
        os.path.basename(part.strip("'\"")) in github_names for word in words for part in TOKEN_SEPARATORS.split(word) if part
    )


def parsed_commands(script: str) -> list[list[str]] | None:
    lexer = shlex.shlex(script, posix=True, punctuation_chars=SHELL_PUNCTUATION)
    lexer.whitespace = " \t\r"
    lexer.whitespace_split = True
    try:
        words = list(lexer)
    except ValueError:
        return None
    commands: list[list[str]] = [[]]
    for word in words:
        if word and all(character in SHELL_PUNCTUATION for character in word):
            commands.append([])
        else:
            commands[-1].append(word)
    return [command for command in commands if command]


def relevant_words(words: list[str], cwd: str, depth: int) -> bool:
    if any(word in SPLIT_STRING_OPTIONS or word.startswith("--split-string=") for word in words) and names_github_program(words):
        return True
    command = unwrap(words)
    if not command:
        return False
    name = os.path.basename(command[0])
    rest = command[1:]
    if any(character in name for character in DYNAMIC_CHARACTERS):
        return True
    if name == "gh" or name in PUSH_FAMILY_PROGRAMS:
        return True
    if name == "git":
        subcommand, _, overrides_alias = git_subcommand(rest)
        if overrides_alias or subcommand in PUSH_FAMILY_SUBCOMMANDS:
            return True
        return bool(subcommand) and subcommand not in GIT_BUILTINS and git_alias_exists(subcommand, cwd)
    if name in SHELLS:
        script = shell_command_argument(rest)
        if script is not None:
            return relevant_script(script, cwd, depth + 1)
        return names_github_program(rest)
    if name in OPAQUE_PROGRAMS:
        return names_github_program(rest)
    return False


def relevant_script(script: str, cwd: str, depth: int = 0) -> bool:
    if depth > MAX_NESTING:
        return True
    commands = parsed_commands(script)
    if commands is None:
        return names_github_program([script])
    return any(relevant_words(words, cwd, depth) for words in commands)


def relevant_command(command: str, cwd: str) -> bool:
    outer, bodies = split_heredocs(command)
    scripts = [outer]
    scripts.extend(match.group(1) or match.group(2) or "" for match in SUBSTITUTION.finditer(without_single_quoted(outer)))
    scripts.extend(body for body, _, opener in bodies if opener_is_shell(opener))
    if any(relevant_script(script, cwd) for script in scripts):
        return True
    return any(
        names_github_program(body.splitlines())
        for body, _, opener in bodies
        if opener_is_interpreter(opener) and not opener_is_shell(opener)
    )


def repository_root(cwd: str) -> str | None:
    return git_read(cwd, "rev-parse", "--show-toplevel")


def commit_id(root: str, revision: str) -> str | None:
    return git_read(root, "rev-parse", "--verify", "--quiet", f"{revision}^{{commit}}")


def is_executable_file(path: str) -> bool:
    return os.path.isfile(path) and os.access(path, os.X_OK)


def main_worktree(root: str) -> str | None:
    common = git_read(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
    if common is None or os.path.basename(common.rstrip("/")) != ".git":
        return None
    parent = os.path.dirname(common.rstrip("/"))
    return parent if os.path.isdir(parent) else None


def scanner_path(root: str) -> str | None:
    main_root = main_worktree(root)
    candidate = os.path.join(main_root if main_root is not None else root, "bin", "gitleaks")
    return candidate if is_executable_file(candidate) else None


def main_file_copy(root: str, path: str, suffix: str, required: bool) -> str | None:
    try:
        result = subprocess.run(
            ["git", "show", f"refs/heads/main:{path}"], cwd=root, capture_output=True, timeout=GIT_TIMEOUT_SECONDS,
            env=child_environment(), check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    if result.returncode != 0 and required:
        return None
    content = result.stdout if result.returncode == 0 else b""
    with tempfile.NamedTemporaryFile("wb", prefix="guard-gitleaks-", suffix=suffix, delete=False) as handle:
        handle.write(content)
    return handle.name


def main_scanner_config(root: str) -> str | None:
    return main_file_copy(root, ".gitleaks.toml", ".toml", True)


def scan_refusal(root: str, base_reference: str, reference: str) -> str | None:
    scanner = scanner_path(root)
    if scanner is None:
        return "bin/gitleaks is missing, so the branch could not be scanned for secrets"
    config = main_scanner_config(root)
    if config is None:
        return "main's .gitleaks.toml could not be read, so the branch could not be scanned for secrets"
    ignored = main_file_copy(root, ".gitleaksignore", ".gitleaksignore", False)
    if ignored is None:
        os.unlink(config)
        return "main's .gitleaksignore could not be read, so the branch could not be scanned for secrets"
    source = git_read(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
    if source is None:
        os.unlink(config)
        os.unlink(ignored)
        return "the repository's Git folder could not be found, so the branch could not be scanned for secrets"
    try:
        result = subprocess.run(
            [
                scanner, "git", "--redact", "--no-banner", "--config", config, "--gitleaks-ignore-path", ignored,
                f"--log-opts={base_reference}..{reference}", source,
            ],
            cwd=root, capture_output=True, timeout=SCANNER_TIMEOUT_SECONDS, env=child_environment(), check=False,
        )
    except subprocess.TimeoutExpired:
        return f"bin/gitleaks did not finish within {SCANNER_TIMEOUT_SECONDS} seconds"
    except (OSError, subprocess.SubprocessError):
        return "bin/gitleaks could not scan the branch"
    finally:
        os.unlink(config)
        os.unlink(ignored)
    return None if result.returncode == 0 else "bin/gitleaks did not pass on the branch"


def owned_regular_file(path: str) -> bool:
    try:
        information = os.lstat(path)
    except OSError:
        return False
    return stat.S_ISREG(information.st_mode) and information.st_uid == os.getuid()


def owned_lines(path: str) -> list[str] | None:
    if not owned_regular_file(path):
        return None
    try:
        with open(path, encoding="utf-8") as handle:
            return handle.read().splitlines()
    except (OSError, UnicodeDecodeError):
        return None


def read_lock(lock_dir: str) -> tuple[str, dict[str, str]]:
    path = os.path.join(lock_dir, LOCK_NAME)
    if not os.path.lexists(path):
        return "absent", {}
    if os.path.islink(lock_dir):
        return "unreadable", {}
    lines = owned_lines(path)
    if lines is None:
        return "unreadable", {}
    fields: dict[str, str] = {}
    for line in lines:
        key, separator, value = line.partition("=")
        if not separator or not LOCK_KEY.match(key) or key in fields:
            return "unreadable", {}
        fields[key] = value
    return ("read", fields) if fields.get("state") in LOCK_STATES else ("unreadable", {})


def lock_for_repository(root: str, lock: dict[str, str]) -> bool:
    common = git_read(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
    return common is not None and lock.get("common_dir") == common


def last_finish_record(lock_dir: str) -> dict[str, str] | None:
    if os.path.islink(lock_dir):
        return None
    lines = owned_lines(os.path.join(lock_dir, LOG_NAME))
    if lines is None:
        return None
    for line in reversed(lines):
        if not line.startswith(FINISH_PREFIX):
            continue
        fields: dict[str, str] = {}
        for word in line[len(FINISH_PREFIX) :].split(" "):
            key, separator, value = word.partition("=")
            if not separator or key in fields:
                return {}
            fields[key] = value
        if set(fields) != FINISH_KEYS or fields["result"] != FINISH_PASSED:
            return {}
        return fields
    return None


def tree_of(root: str, sha: str) -> str | None:
    return git_read(root, "rev-parse", "--verify", "--quiet", f"{sha}^{{tree}}")


def only_parent(root: str, sha: str) -> str | None:
    parents = git_read(root, "rev-list", "--parents", "-n", "1", sha)
    words = parents.split() if parents else []
    return words[1] if len(words) == 2 and words[0] == sha else None


def branch_names_at(root: str, sha: str) -> list[str] | None:
    listing = git_read(root, "for-each-ref", "--format=%(objectname) %(refname)", "--points-at", sha, "refs/heads")
    if listing is None:
        return None
    names: list[str] = []
    for line in listing.splitlines():
        objectname, _, refname = line.partition(" ")
        if objectname == sha and refname.startswith("refs/heads/"):
            names.append(refname[len("refs/heads/") :])
    return names


def is_gate_branch_name(name: str) -> bool:
    return bool(BRANCH_NAME.match(name)) and not name.endswith(EVIDENCE_ENDINGS)


def is_rebuild_of(name: str, change_id: str) -> bool:
    if not is_gate_branch_name(change_id) or not name.startswith(change_id):
        return False
    suffix = name[len(change_id) :]
    return not suffix or bool(REBUILD_SUFFIX.match(suffix))


def is_evidence_branch_of(name: str, change_id: str, ending: str) -> bool:
    return name.endswith(ending) and is_rebuild_of(name[: -len(ending)], change_id)


def evidence_selection(inputs: dict[str, str]) -> str:
    title = inputs.get("title", "")
    digest = hashlib.sha256(title.encode("utf-8")).hexdigest() if title else ""
    parts = [
        f"kind:{inputs.get('kind', '')}", f"package:{inputs.get('package', '')}", f"test:{inputs.get('test', '')}",
        f"project:{inputs.get('project', '')}", f"title-sha256:{digest}", f"entry:{inputs.get('entry', '')}",
    ]
    return "|".join(parts)


def reserved_email_domain(domain: str) -> bool:
    lowered = domain.lower()
    labels = lowered.split(".")
    if labels[-1] in RESERVED_TOP_LEVEL_DOMAINS or (len(labels) >= 2 and labels[-2] == RESERVED_SECOND_LEVEL_LABEL):
        return True
    return lowered == NOREPLY_DOMAIN or lowered.endswith(f".{NOREPLY_DOMAIN}")


def holds_personal_data(text: str) -> bool:
    if RUSSIAN_MOBILE.search(text):
        return True
    return any(not reserved_email_domain(match.group(1)) for match in EMAIL_ADDRESS.finditer(text))


def git_log_text(root: str, arguments: list[str]) -> str | None:
    try:
        result = subprocess.run(
            ["git", "log", "--no-color", *arguments], cwd=root, capture_output=True, text=True, errors="replace",
            timeout=GIT_TIMEOUT_SECONDS, env=child_environment(), check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    return result.stdout if result.returncode == 0 else None


def personal_data_refusal(root: str, base_reference: str, reference: str) -> str | None:
    revisions = f"{base_reference}..{reference}"
    messages = git_log_text(root, ["--format=%B", revisions])
    diff = git_log_text(root, ["--no-ext-diff", "--no-textconv", "--format=", "-p", "-U0", revisions])
    if messages is None or diff is None:
        return "the branch could not be read for the personal-data scan"
    added = [line[1:] for line in diff.splitlines() if line.startswith("+") and not line.startswith(DIFF_HEADERS)]
    if any(holds_personal_data(line) for line in messages.splitlines() + added):
        return (
            "the heuristic personal-data scan found an email address outside the reserved test domains "
            "or a Russian mobile phone number in the commits to push"
        )
    return None


def branch_push_refusal(root: str, branch: str) -> str | None:
    reference = f"refs/heads/{branch}"
    main_commit = commit_id(root, "refs/heads/main")
    branch_parent = commit_id(root, f"{reference}^1")
    count = git_read(root, "rev-list", "--count", f"refs/heads/main..{reference}")
    merges = git_read(root, "rev-list", "--count", "--min-parents=2", f"refs/heads/main..{reference}")
    if main_commit is None or branch_parent != main_commit or count != "1" or merges != "0":
        return "the branch is not exactly one commit on local main"
    branch_workflows = git_read(root, "rev-parse", "--verify", "--quiet", f"{reference}:.github")
    main_workflows = git_read(root, "rev-parse", "--verify", "--quiet", "refs/heads/main:.github")
    if branch_workflows is None or branch_workflows != main_workflows:
        return "the branch's .github differs from main's"
    return scan_refusal(root, "refs/heads/main", reference) or personal_data_refusal(root, "refs/heads/main", reference)


def catch_up_push_refusal(root: str, branch: str, lock_dir: str) -> str | None:
    reference = f"refs/heads/{branch}"
    status, lock = read_lock(lock_dir)
    if status != "read" or lock.get("state") != "dispatched" or lock.get("kind") != CATCH_UP_KIND:
        return "a catch-up push without a merge-gate catch-up claim (state=dispatched, kind=catch-up)"
    if not lock_for_repository(root, lock):
        return "the merge-gate claim belongs to another repository"
    tip = commit_id(root, reference)
    origin = commit_id(root, "refs/remotes/origin/main")
    if tip is None or tip != lock.get("head") or tip != commit_id(root, "refs/heads/main") or branch != f"catch-up/{tip[:12]}":
        return "the catch-up branch is not local main's tip, the claimed commit and named by its first 12 characters"
    if origin is None or only_parent(root, tip) != origin or lock.get("base") != origin:
        return "the catch-up commit's only parent is not origin/main"
    branch_workflows = git_read(root, "rev-parse", "--verify", "--quiet", f"{reference}:.github")
    origin_workflows = git_read(root, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/main:.github")
    if branch_workflows is None or branch_workflows != origin_workflows:
        return "the catch-up commit changes .github/, which the owner pushes"
    return scan_refusal(root, "refs/remotes/origin/main", reference) or personal_data_refusal(
        root, "refs/remotes/origin/main", reference
    )


def main_push_refusal(root: str, lock_dir: str) -> str | None:
    main_commit = commit_id(root, "refs/heads/main")
    origin_commit = commit_id(root, "refs/remotes/origin/main")
    if main_commit is None or origin_commit is None:
        return "main or origin/main could not be read"
    if git_read(root, "rev-list", "--count", "refs/heads/main..refs/remotes/origin/main") != "0":
        return "origin/main is not an ancestor of local main"
    changed = git_read(root, "rev-list", "refs/remotes/origin/main..refs/heads/main", "--", ".github")
    if changed is None or changed:
        return "a commit to push changes .github/, which the owner pushes"
    if only_parent(root, main_commit) != origin_commit:
        return "local main is not exactly one finished merge on origin/main"
    record = last_finish_record(lock_dir)
    if record is None:
        return "the merge-gate log holds no finish line"
    if not record:
        return "the merge-gate log's last finish line is not exactly record, head, tree and result=passed"
    if record.get("record") not in CI_RECORDS:
        return "the merge-gate log's last finish line is not a ci or ci-catch-up record, so the owner pushes main"
    if record.get("head") != main_commit or record.get("tree") != tree_of(root, main_commit):
        return "the merge-gate log's last finish line does not name local main's tip and its tree"
    return None


def push_refusal(arguments: list[str], cwd: str, lock_dir: str) -> str | None:
    if len(arguments) != 2 or arguments[0] != "origin":
        return "a push outside the exact branch and main forms"
    source, separator, target = arguments[1].partition(":")
    if not separator or source != target or not source.startswith("refs/heads/"):
        return "a push outside the exact branch and main forms"
    branch = source[len("refs/heads/") :]
    if branch != "main" and not BRANCH_NAME.match(branch) and not CATCH_UP_BRANCH.match(branch):
        return "a push of a branch outside the feat/, fix/, chore/, docs/ and catch-up/ names"
    root = repository_root(cwd)
    if root is None:
        return "the repository could not be read"
    if branch == "main":
        return main_push_refusal(root, lock_dir)
    if CATCH_UP_BRANCH.match(branch):
        return catch_up_push_refusal(root, branch, lock_dir)
    return branch_push_refusal(root, branch)


def option_value(arguments: list[str], index: int) -> tuple[str, str | None, int]:
    word = arguments[index]
    if word.startswith("--") and "=" in word:
        name, _, value = word.partition("=")
        return name, value, index + 1
    return word, arguments[index + 1] if index + 1 < len(arguments) else None, index + 2


def config_is_read(arguments: list[str]) -> bool:
    positional: list[str] = []
    flags: set[str] = set()
    index = 0
    while index < len(arguments):
        word = arguments[index]
        if not word.startswith("-"):
            positional.append(word)
            index += 1
            continue
        name, value, following = option_value(arguments, index)
        if name in CONFIG_OPTIONS_WITH_VALUES:
            if value is None:
                return False
            index = following
            continue
        if name not in CONFIG_READ_FLAGS | CONFIG_NEUTRAL_FLAGS or "=" in word:
            return False
        flags.add(name)
        index += 1
    if positional and positional[0] in CONFIG_WRITE_SUBCOMMANDS:
        return False
    if positional and positional[0] in CONFIG_READ_SUBCOMMANDS:
        return True
    if flags & CONFIG_READ_FLAGS:
        return True
    return len(positional) == 1


def remote_is_read(arguments: list[str]) -> bool:
    if arguments in ([], ["-v"], ["--verbose"]):
        return True
    if arguments[:1] == ["get-url"]:
        rest = arguments[1:]
        names = [word for word in rest if not word.startswith("-")]
        return len(names) == 1 and {word for word in rest if word.startswith("-")} <= REMOTE_GET_URL_FLAGS
    return False


def git_form_refusal(arguments: list[str], cwd: str, lock_dir: str) -> str | None:
    if arguments[:1] == ["push"]:
        return push_refusal(arguments[1:], cwd, lock_dir)
    if arguments[:1] == ["config"]:
        return None if config_is_read(arguments[1:]) else "a git config write"
    if arguments[:1] == ["remote"]:
        return None if remote_is_read(arguments[1:]) else "a git remote write"
    return "a git push, send-pack, remote or config call outside the allowed forms, or a git option or alias in front of it"


def dispatch_inputs(arguments: list[str]) -> dict[str, str] | None:
    if len(arguments) < 7 or arguments[:3] != ["gate.yml", "--ref", "main"] or (len(arguments) - 3) % 2:
        return None
    inputs: dict[str, str] = {}
    for index in range(3, len(arguments), 2):
        if arguments[index] != "-f":
            return None
        name, separator, value = arguments[index + 1].partition("=")
        if not separator or not value or not DISPATCH_INPUT_NAME.match(name) or name in inputs:
            return None
        inputs[name] = value
    return inputs


def valid_jobs(value: str) -> bool:
    seen: set[str] = set()
    for item in value.split(","):
        name, separator, shard = item.partition(":")
        if name not in CI_JOB_SHARDS or item in seen:
            return False
        if separator and (not SHARD_NUMBER.match(shard) or int(shard) > CI_JOB_SHARDS[name]):
            return False
        seen.add(item)
    return bool(seen)


def evidence_inputs_refusal(inputs: dict[str, str]) -> str | None:
    selection = EVIDENCE_SELECTIONS.get(inputs.get("kind", ""))
    if selection is None or inputs.get("series") not in EVIDENCE_BRANCH_SUFFIXES:
        return "an evidence dispatch outside the go, playwright and check kinds and the before-first, before-rest and after series"
    if set(inputs) != {"sha", "scope", "kind", "series", *selection}:
        return "an evidence dispatch with inputs outside its kind's selection"
    if not all(EVIDENCE_VALUES[name].match(inputs[name]) for name in selection):
        return "an evidence dispatch whose selection is outside the allowed characters"
    return None


def gate_dispatch_refusal(sha: str, scope: str, root: str, lock_dir: str) -> str | None:
    status, lock = read_lock(lock_dir)
    if status != "read" or lock.get("state") != "dispatched" or lock.get("head") != sha:
        return "a full or docs dispatch without a merge-gate claim of that commit (state=dispatched); only the coordinator dispatches them, after merge-gate.sh claim"
    if not lock_for_repository(root, lock):
        return "the merge-gate claim belongs to another repository"
    if lock.get("tree") != tree_of(root, sha):
        return "the dispatched commit's tree differs from the claimed tree"
    names = branch_names_at(root, sha)
    if names is None:
        return "the local branches could not be read"
    if lock.get("kind") == CATCH_UP_KIND:
        origin = commit_id(root, "refs/remotes/origin/main")
        if scope != "full":
            return "a catch-up run is a full run"
        if origin is None or only_parent(root, sha) != origin or lock.get("base") != origin:
            return "the catch-up commit's only parent is not origin/main"
        if f"catch-up/{sha[:12]}" not in names:
            return "the catch-up commit is not the tip of the local branch catch-up/<its first 12 characters>"
        return None
    if lock.get("kind") not in GATE_KINDS:
        return "the merge-gate claim has an unknown kind"
    main_commit = commit_id(root, "refs/heads/main")
    if main_commit is None or only_parent(root, sha) != main_commit or lock.get("base") != main_commit:
        return "the dispatched commit is not one commit whose only parent is local main's tip and the claimed base"
    change_id = lock.get("change_id", "")
    if not any(is_rebuild_of(name, change_id) for name in names):
        return "the dispatched commit is not the tip of a local branch named <change-id> or <change-id>-r<k> after the claim"
    return None


def change_dispatch_refusal(sha: str, root: str, lock_dir: str) -> str | None:
    status, lock = read_lock(lock_dir)
    if status == "unreadable":
        return "the merge-gate lock could not be read, so a gate run may be claimed"
    if status == "read" and lock.get("state") == "dispatched":
        return "a change dispatch while a gate run is claimed (state=dispatched)"
    main_commit = commit_id(root, "refs/heads/main")
    if main_commit is None or only_parent(root, sha) != main_commit:
        return "the dispatched commit is not one commit whose only parent is local main's tip"
    names = branch_names_at(root, sha)
    if names is None:
        return "the local branches could not be read"
    if not any(is_gate_branch_name(name) for name in names):
        return "the dispatched commit is not the tip of a local feat/, fix/, chore/ or docs/ branch named <change-id> or <change-id>-r<k>"
    return None


def evidence_dispatch_refusal(sha: str, inputs: dict[str, str], root: str, lock_dir: str) -> str | None:
    series = inputs["series"]
    status, lock = read_lock(lock_dir)
    if status != "read" or lock.get("state") != "evidence":
        return "an evidence dispatch without an open evidence series (state=evidence)"
    if not lock_for_repository(root, lock):
        return "the evidence series belongs to another repository"
    if lock.get("pending") != f"{series},{sha}":
        return "the evidence dispatch does not match the one dispatch the series holds open (pending)"
    if not lock.get("selection") or lock.get("selection") != evidence_selection(inputs):
        return "the evidence dispatch's selection inputs differ from the selection the series claimed (selection)"
    base = lock.get("base", "")
    if not base or only_parent(root, sha) != base:
        return "the evidence commit's only parent is not the series base"
    if series in BEFORE_SERIES and tree_of(root, sha) != lock.get("before_tree"):
        return "the before commit's tree differs from the series' before tree"
    change_id = lock.get("change_id", "")
    names = branch_names_at(root, sha)
    if names is None:
        return "the local branches could not be read"
    ending = EVIDENCE_BRANCH_SUFFIXES[series]
    if not is_gate_branch_name(change_id) or not any(is_evidence_branch_of(name, change_id, ending) for name in names):
        return "the evidence commit is not the tip of a local branch <change-id>[-r<k>]-before or <change-id>[-r<k>]-after"
    return None


def workflow_run_refusal(arguments: list[str], cwd: str, lock_dir: str) -> str | None:
    inputs = dispatch_inputs(arguments)
    if inputs is None or not FULL_SHA.match(inputs.get("sha", "")):
        return "a workflow dispatch outside the exact gate.yml form"
    sha = inputs["sha"]
    scope = inputs.get("scope", "")
    if scope in GATE_SCOPES and set(inputs) != {"sha", "scope"}:
        return "a workflow dispatch outside the exact gate.yml form"
    if scope == CHANGE_SCOPE and (set(inputs) != {"sha", "scope", "jobs"} or not valid_jobs(inputs["jobs"])):
        return "a change dispatch whose jobs are not a list of CI jobs and shards"
    if scope == EVIDENCE_SCOPE:
        refusal = evidence_inputs_refusal(inputs)
        if refusal:
            return refusal
    if scope not in GATE_SCOPES | {CHANGE_SCOPE, EVIDENCE_SCOPE}:
        return "a workflow dispatch outside the exact gate.yml form"
    root = repository_root(cwd)
    if root is None:
        return "the repository could not be read"
    if scope == CHANGE_SCOPE:
        return change_dispatch_refusal(sha, root, lock_dir)
    if scope == EVIDENCE_SCOPE:
        return evidence_dispatch_refusal(sha, inputs, root, lock_dir)
    return gate_dispatch_refusal(sha, scope, root, lock_dir)


def pull_request_refusal(subcommand: str, arguments: list[str]) -> str | None:
    with_values = PR_OPTIONS_WITH_VALUES[subcommand]
    positional: list[str] = []
    fields: str | None = None
    index = 0
    while index < len(arguments):
        word = arguments[index]
        if not word.startswith("-"):
            positional.append(word)
            index += 1
            continue
        name, value, following = option_value(arguments, index)
        if name not in with_values or value is None:
            return "a pull request read with an option outside the allowed set"
        if name == "--json":
            fields = value
        index = following
    if fields is None or not set(fields.split(",")) <= PR_FIELDS or "" in fields.split(","):
        return "a pull request read without --json limited to number, state, head and URL"
    allowed_positional = 1 if subcommand == "view" else 0
    if len(positional) > allowed_positional or not all(PR_SELECTOR.match(word) for word in positional):
        return "a pull request read with an unexpected selector"
    return None


def api_refusal(arguments: list[str]) -> str | None:
    paths: list[str] = []
    index = 0
    while index < len(arguments):
        word = arguments[index]
        if not word.startswith("-"):
            paths.append(word)
            index += 1
            continue
        if word in API_FLAGS:
            index += 1
            continue
        if word.startswith(("-X", "-H")) and len(word) > 2:
            name, value, following = word[:2], word[2:], index + 1
        else:
            name, value, following = option_value(arguments, index)
        if value is None:
            return "a GitHub API call with an incomplete option"
        if name in {"-X", "--method"}:
            if value.upper() != "GET":
                return "a GitHub API call with a method other than GET"
        elif name in {"-H", "--header"}:
            if not ACCEPT_HEADER.match(value):
                return "a GitHub API call with a header other than Accept"
        elif name not in API_OPTIONS_WITH_VALUES:
            return "a GitHub API call with an option outside the allowed set"
        index = following
    if len(paths) != 1 or not API_PATH.match(paths[0]):
        return "a GitHub API call outside the allowed repos/klukashka/ovc paths"
    return None


def gh_form_refusal(arguments: list[str], cwd: str, lock_dir: str) -> str | None:
    for word in arguments:
        if word.startswith(GH_REPOSITORY_OPTIONS):
            return "a GitHub CLI call with --repo or --hostname"
    if arguments[:2] == ["workflow", "run"]:
        return workflow_run_refusal(arguments[2:], cwd, lock_dir)
    if len(arguments) >= 2 and arguments[0] == "run" and arguments[1] in GH_RUN_READS:
        return None
    if len(arguments) >= 2 and arguments[0] == "pr" and arguments[1] in PR_OPTIONS_WITH_VALUES:
        return pull_request_refusal(arguments[1], arguments[2:])
    if arguments[:1] == ["api"]:
        return api_refusal(arguments[1:])
    return "a GitHub CLI call outside the allowed forms"


def allowed_form_refusal(command: str, cwd: str, lock_dir: str) -> str | None:
    if any(character in command for character in UNSAFE_CHARACTERS):
        return "a command with substitution, escapes or several lines"
    if any(word and all(character in SHELL_PUNCTUATION for character in word) for word in shell_words(command)):
        return "a compound command, pipe, redirection or subshell"
    try:
        words = shlex.split(command, posix=True)
    except ValueError:
        return "a command the guard could not parse"
    if not words:
        return "a command the guard could not parse"
    if words[0] == "git":
        return git_form_refusal(words[1:], cwd, lock_dir)
    if words[0] == "gh":
        return gh_form_refusal(words[1:], cwd, lock_dir)
    return "an indirect call of git or gh (wrapper, environment prefix, path or shell)"


def bash_token_lists(command: str) -> list[list[str]]:
    outer, bodies = split_heredocs(command)
    scripts = [outer]
    scripts.extend(match.group(1) or match.group(2) or "" for match in SUBSTITUTION.finditer(without_single_quoted(outer)))
    scripts.extend(body for body, _, opener in bodies if opener_is_shell(opener))
    lists: list[list[str]] = []
    for script in scripts:
        for words in simple_commands(script):
            if not is_safe_git(" ".join(shlex.quote(word) for word in words)):
                lists.append(flat_tokens(words))
    for body, _, opener in bodies:
        if opener_is_interpreter(opener) and not opener_is_shell(opener):
            lists.extend([token.strip("'\"") for token in command_tokens(line)] for line in body.splitlines())
    return lists


def leading_operands(arguments: list[str], with_values: set[str]) -> list[str]:
    index = 0
    while index < len(arguments) and arguments[index].startswith("-"):
        word = arguments[index]
        if word == "--":
            return arguments[index + 1 :]
        index += 2 if word in with_values else 1
    return arguments[index:]


def runner_script(arguments: list[str], with_values: set[str]) -> str | None:
    rest = leading_operands(arguments, with_values)
    if rest and rest[0] in SCRIPT_RUNNERS:
        rest = leading_operands(rest[1:], with_values)
    return rest[0] if rest else None


def is_check_script(name: str | None) -> bool:
    return name is not None and (name in PNPM_CHECK_SCRIPTS or name.startswith(PNPM_CHECK_PREFIXES))


def script_check_reason(path: str, arguments: list[str]) -> str | None:
    name = os.path.basename(path)
    if name == MERGE_GATE_SCRIPT:
        subcommands = sorted(set(arguments) & MERGE_GATE_CHECK_SUBCOMMANDS)
        return f"a {MERGE_GATE_SCRIPT} {subcommands[0]} run" if subcommands else None
    if name == GUARD_CHECKER:
        return "a run of the guard's own checker"
    if any(fnmatch.fnmatch(name, pattern) for pattern in CHECK_SCRIPT_PATTERNS):
        return f"a local check script ({name})"
    return None


def program_check_reason(program: str, arguments: list[str], depth: int) -> str | None:
    if program in CHECK_PROGRAMS:
        return f"a local check ({program})"
    if program == "go":
        rest = leading_operands(arguments, GO_OPTIONS_WITH_VALUES)
        if rest and rest[0] in GO_CHECK_SUBCOMMANDS:
            return f"a local check (go {rest[0]})"
        if rest and rest[0] in GO_TOOL_RUNNERS and any(tool in word for word in rest[1:] for tool in CHECK_TOOL_NAMES):
            return "a local check through go run or go tool"
        return None
    if program == "gofmt" and set(arguments) & GOFMT_CHECK_FLAGS and "-w" not in arguments:
        return "a local check (gofmt without -w)"
    if program == "sqlc":
        rest = leading_operands(arguments, SQLC_OPTIONS_WITH_VALUES)
        return f"a local check (sqlc {rest[0]})" if rest and rest[0] in SQLC_CHECK_SUBCOMMANDS else None
    if program == "prettier" and set(arguments) & PRETTIER_CHECK_FLAGS:
        return "a local check (prettier --check)"
    if program == "playwright" and leading_operands(arguments, set())[:1] == ["test"]:
        return "a local check (playwright test)"
    if program == "pnpm":
        rest = leading_operands(arguments, PNPM_OPTIONS_WITH_VALUES)
        if rest and rest[0] in PNPM_EXEC_COMMANDS:
            return package_program_reason(leading_operands(rest[1:], PNPM_OPTIONS_WITH_VALUES), depth)
        script = runner_script(arguments, PNPM_OPTIONS_WITH_VALUES)
        return f"a local check (pnpm {script})" if is_check_script(script) else None
    if program in NPX_PROGRAMS:
        return package_program_reason(leading_operands(arguments, NPM_OPTIONS_WITH_VALUES), depth)
    if program == "npm":
        rest = leading_operands(arguments, NPM_OPTIONS_WITH_VALUES)
        if rest and rest[0] in NPM_EXEC_COMMANDS:
            return package_program_reason(leading_operands(rest[1:], NPM_OPTIONS_WITH_VALUES), depth)
        script = runner_script(rest, NPM_OPTIONS_WITH_VALUES)
        return f"a local check (npm {script})" if is_check_script(script) else None
    return None


def package_program(word: str) -> str:
    if word.startswith("@"):
        _, slash, rest = word.rpartition("/")
        name = rest.split("@", 1)[0] if slash else ""
    else:
        name = "" if "/" in word else word.split("@", 1)[0]
    return name or word


def package_program_reason(words: list[str], depth: int) -> str | None:
    if not words:
        return None
    return check_words_reason([package_program(words[0]), *words[1:]], depth + 1)


def check_words_reason(words: list[str], depth: int) -> str | None:
    if depth > MAX_NESTING:
        return "a command nested too deeply to check"
    command = unwrap(words)
    if not command:
        return None
    program = os.path.basename(command[0])
    arguments = command[1:]
    found = script_check_reason(command[0], arguments) or program_check_reason(program, arguments, depth)
    if found:
        return found
    if program in SHELLS:
        inner = shell_command_argument(arguments)
        if inner is not None:
            return check_script_reason(inner, depth + 1)
        script = shell_script(arguments)
        if script is None:
            return None
        found = script_check_reason(script[0], script[1])
        if found or os.path.basename(script[0]) != SETTINGS_WRAPPER:
            return found
        return check_words_reason(script[1], depth + 1)
    if program == "eval":
        return check_script_reason(" ".join(arguments), depth + 1)
    if (program.startswith("python") or program in INTERPRETER_PROGRAMS) and MODULE_OPTION not in arguments:
        if any(os.path.basename(word) == GUARD_CHECKER for word in arguments):
            return "a run of the guard's own checker"
    return None


def check_script_reason(script: str, depth: int) -> str | None:
    if depth > MAX_NESTING:
        return "a command nested too deeply to check"
    for words in simple_commands(script):
        found = check_words_reason(words, depth)
        if found:
            return found
    return None


def local_check_reason(command: str) -> str | None:
    outer, bodies = split_heredocs(command)
    scripts = [outer]
    scripts.extend(match.group(1) or match.group(2) or "" for match in SUBSTITUTION.finditer(without_single_quoted(outer)))
    scripts.extend(body for body, _, opener in bodies if opener_is_shell(opener))
    for script in scripts:
        found = check_script_reason(script, 0)
        if found:
            return found
    return None


def default_lock_dir() -> str:
    home = pwd.getpwuid(os.getuid()).pw_dir
    if sys.platform == "darwin":
        return os.path.join(home, "Library", "Caches", "ovc-e2e")
    return os.path.join(home, ".cache", "ovc-e2e")


def lock_folder(arguments: list[str]) -> str:
    if not arguments:
        return default_lock_dir()
    if len(arguments) == 2 and arguments[0] == LOCK_DIR_OPTION and os.path.isabs(arguments[1]):
        return arguments[1]
    raise ValueError("unexpected hook arguments")


def bash_decision(command: str, cwd: str, lock_dir: str) -> tuple[str, str] | None:
    lists = bash_token_lists(command)
    for tokens in lists:
        denial = denied_call(tokens)
        if denial:
            return "deny", denial
    local_check = local_check_reason(command)
    if relevant_command(command, cwd):
        if split_heredocs(command)[1]:
            return "ask", "a git push-family or gh call with a heredoc"
        refusal = allowed_form_refusal(command, cwd, lock_dir)
        if refusal is not None:
            return "ask", refusal
    if local_check is not None:
        return "ask", f"{local_check}, which runs in CI rather than on the laptop"
    return None


def credential_file_path(path: str, tool: str) -> bool:
    expanded = os.path.expanduser(path.strip().strip("'\""))
    for candidate in {expanded, os.path.realpath(expanded)}:
        cleaned = candidate.rstrip("/")
        if cleaned.endswith(f"/{GH_HOSTS_FILE}") or (tool in SEARCH_TOOLS and f"/{GH_CONFIG_DIRECTORY}" in f"{cleaned}/"):
            return True
    return False


def github_decision(payload: dict[str, object], lock_dir: str) -> tuple[str, str] | None:
    tool = payload.get("tool_name")
    tool_input = payload.get("tool_input")
    if not isinstance(tool, str) or not isinstance(tool_input, dict):
        return "ask", "the guard could not read the hook payload"
    if tool in FILE_TOOLS or tool in SEARCH_TOOLS:
        for key in ("file_path", "notebook_path", "path"):
            value = tool_input.get(key)
            if isinstance(value, str) and credential_file_path(value, tool):
                return "deny", "a read of the GitHub CLI credential file"
        return None
    if tool != "Bash":
        return None
    command = tool_input.get("command")
    cwd = payload.get("cwd")
    if not isinstance(command, str):
        return "ask", "the guard could not read the command"
    return bash_decision(command, cwd if isinstance(cwd, str) and os.path.isdir(cwd) else os.getcwd(), lock_dir)


def ask_owner(reason: str) -> int:
    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "ask",
                    "permissionDecisionReason": f"The guard asks the owner: {reason}. Only the exact allowed push, dispatch and read forms pass without asking.",
                }
            }
        )
    )
    return 0


def credential_refusal(reason: str) -> int:
    print(
        f"Blocked: {reason}. Agents never read, print, store or switch GitHub or Git credentials in this repository; "
        "ask the owner to run it if it is really needed.",
        file=sys.stderr,
    )
    return 2


def main() -> int:
    try:
        payload = json.load(sys.stdin)
        target = blocked_target(payload) if isinstance(payload, dict) else "malformed hook payload"
    except Exception as error:
        target = f"hook failure ({type(error).__name__})"
    if target is None:
        try:
            decision = github_decision(payload, lock_folder(sys.argv[1:]))
        except Exception as error:
            decision = ("ask", f"the guard could not check the call ({type(error).__name__})")
        if decision is None:
            return 0
        kind, reason = decision
        return credential_refusal(reason) if kind == "deny" else ask_owner(reason)
    if target.startswith(("a command", "a container", "a read", "an interpreter", "a traced", "a shell")):
        print(
            f"Blocked: {target}. Agents never print the environment or credentials in this repository; "
            "ask the owner to run it if it is really needed.",
            file=sys.stderr,
        )
        return 2
    print(
        f"Blocked: {target} looks like a secret (environment file, key or credential directory), "
        "or the guard could not check the call. Agents never read or edit secrets in this repository; ask the owner instead.",
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    sys.exit(main())
