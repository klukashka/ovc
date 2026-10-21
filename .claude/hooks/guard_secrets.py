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
}
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
TRACE_VARIABLES = ("SHELLOPTS=", "BASHOPTS=", "BASH_ENV=", "BASH_XTRACEFD=")


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
    before = words[:position]
    if any(word.startswith(TRACE_VARIABLES) for word in before):
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


def words_printer(words: list[str], depth: int) -> str | None:
    if depth > MAX_NESTING:
        return "a command nested too deeply to check"
    traced = settings_wrapper_trace(words)
    if traced:
        return traced
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


def main() -> int:
    try:
        payload = json.load(sys.stdin)
        target = blocked_target(payload) if isinstance(payload, dict) else "malformed hook payload"
    except Exception as error:
        target = f"hook failure ({type(error).__name__})"
    if target is None:
        return 0
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
