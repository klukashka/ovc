#!/usr/bin/env python3
from __future__ import annotations

import fnmatch
import json
import os
import re
import shlex
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
            words = skip_options(program, words[1:])
            continue
        if program == "pnpm":
            rest = skip_options(program, words[1:])
            if rest and rest[0] in PNPM_RUNNERS:
                words = skip_options(f"pnpm-{rest[0]}", rest[1:])
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


def scan_refusal(root: str, reference: str) -> str | None:
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
                f"--log-opts=refs/heads/main..{reference}", source,
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
    return scan_refusal(root, reference)


def main_push_refusal(root: str) -> str | None:
    if commit_id(root, "refs/heads/main") is None or commit_id(root, "refs/remotes/origin/main") is None:
        return "main or origin/main could not be read"
    if git_read(root, "rev-list", "--count", "refs/heads/main..refs/remotes/origin/main") != "0":
        return "origin/main is not an ancestor of local main"
    changed = git_read(root, "rev-list", "refs/remotes/origin/main..refs/heads/main", "--", ".github")
    if changed is None or changed:
        return "a commit to push changes .github/, which the owner pushes"
    return None


def push_refusal(arguments: list[str], cwd: str) -> str | None:
    if len(arguments) != 2 or arguments[0] != "origin":
        return "a push outside the exact branch and main forms"
    source, separator, target = arguments[1].partition(":")
    if not separator or source != target or not source.startswith("refs/heads/"):
        return "a push outside the exact branch and main forms"
    branch = source[len("refs/heads/") :]
    if branch != "main" and not BRANCH_NAME.match(branch):
        return "a push of a branch outside the feat/, fix/, chore/ and docs/ names"
    root = repository_root(cwd)
    if root is None:
        return "the repository could not be read"
    return main_push_refusal(root) if branch == "main" else branch_push_refusal(root, branch)


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


def git_form_refusal(arguments: list[str], cwd: str) -> str | None:
    if arguments[:1] == ["push"]:
        return push_refusal(arguments[1:], cwd)
    if arguments[:1] == ["config"]:
        return None if config_is_read(arguments[1:]) else "a git config write"
    if arguments[:1] == ["remote"]:
        return None if remote_is_read(arguments[1:]) else "a git remote write"
    return "a git push, send-pack, remote or config call outside the allowed forms, or a git option or alias in front of it"


def candidate_refusal(sha: str, cwd: str) -> str | None:
    root = repository_root(cwd)
    if root is None:
        return "the repository could not be read"
    main_commit = commit_id(root, "refs/heads/main")
    parents = git_read(root, "rev-list", "--parents", "-n", "1", sha)
    if main_commit is None or parents != f"{sha} {main_commit}":
        return "the dispatched commit is not one commit whose only parent is local main's tip"
    listing = git_read(
        root, "for-each-ref", "--format=%(objectname) %(refname)", "--points-at", sha,
        "refs/heads/feat", "refs/heads/fix", "refs/heads/chore", "refs/heads/docs",
    )
    if listing is None:
        return "the local branches could not be read"
    for line in listing.splitlines():
        objectname, _, refname = line.partition(" ")
        if objectname == sha and refname.startswith("refs/heads/") and BRANCH_NAME.match(refname[len("refs/heads/") :]):
            return None
    return "the dispatched commit is not the tip of a local feat/, fix/, chore/ or docs/ branch"


def workflow_run_refusal(arguments: list[str], cwd: str) -> str | None:
    if len(arguments) != 7 or arguments[:4] != ["gate.yml", "--ref", "main", "-f"] or arguments[5] != "-f":
        return "a workflow dispatch outside the exact gate.yml form"
    sha_name, _, sha = arguments[4].partition("=")
    scope_name, _, scope = arguments[6].partition("=")
    if sha_name != "sha" or not FULL_SHA.match(sha) or scope_name != "scope" or scope not in GATE_SCOPES:
        return "a workflow dispatch outside the exact gate.yml form"
    return candidate_refusal(sha, cwd)


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


def gh_form_refusal(arguments: list[str], cwd: str) -> str | None:
    for word in arguments:
        if word.startswith(GH_REPOSITORY_OPTIONS):
            return "a GitHub CLI call with --repo or --hostname"
    if arguments[:2] == ["workflow", "run"]:
        return workflow_run_refusal(arguments[2:], cwd)
    if len(arguments) >= 2 and arguments[0] == "run" and arguments[1] in GH_RUN_READS:
        return None
    if len(arguments) >= 2 and arguments[0] == "pr" and arguments[1] in PR_OPTIONS_WITH_VALUES:
        return pull_request_refusal(arguments[1], arguments[2:])
    if arguments[:1] == ["api"]:
        return api_refusal(arguments[1:])
    return "a GitHub CLI call outside the allowed forms"


def allowed_form_refusal(command: str, cwd: str) -> str | None:
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
        return git_form_refusal(words[1:], cwd)
    if words[0] == "gh":
        return gh_form_refusal(words[1:], cwd)
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


def bash_decision(command: str, cwd: str) -> tuple[str, str] | None:
    lists = bash_token_lists(command)
    for tokens in lists:
        denial = denied_call(tokens)
        if denial:
            return "deny", denial
    if not relevant_command(command, cwd):
        return None
    if split_heredocs(command)[1]:
        return "ask", "a git push-family or gh call with a heredoc"
    refusal = allowed_form_refusal(command, cwd)
    return None if refusal is None else ("ask", refusal)


def credential_file_path(path: str, tool: str) -> bool:
    expanded = os.path.expanduser(path.strip().strip("'\""))
    for candidate in {expanded, os.path.realpath(expanded)}:
        cleaned = candidate.rstrip("/")
        if cleaned.endswith(f"/{GH_HOSTS_FILE}") or (tool in SEARCH_TOOLS and f"/{GH_CONFIG_DIRECTORY}" in f"{cleaned}/"):
            return True
    return False


def github_decision(payload: dict[str, object]) -> tuple[str, str] | None:
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
    return bash_decision(command, cwd if isinstance(cwd, str) and os.path.isdir(cwd) else os.getcwd())


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
            decision = github_decision(payload)
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
