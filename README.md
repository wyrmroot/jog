# jog
A lean task runner for concurrent command execution.

Aims to do _less_ than other task launchers, and stays out of your way while scaling up to millions of tasks.
`jog` applies simple controls for *concurrency* and *retry strategy* to any program invocation.
Like `xargs`, `jog` plays nicely with GNU utilities such as `find`, and is designed to have a large number of arguments streamed into it.
A common use case is to apply a program to a large directly of input files without needing to write methods for batch processing within the application code.

## Features
- Run commands _concurrently_ with no changes to application code. These green threads allow processes with downtime (think API calls or database queries) to effectively share CPU time.
- Reasonable options for handling failures (ignore, retry with backoff, abort, etc.)
- No YAML, DSL, DAG, functions, or recipes. There are good workflow orchestrators out there (and bad ones, too). `jog` just runs your commands.
- Optional structured logging
- Minimal execution footprint. Want to run a few million of something? Go for it!
- Supports some of the more useful `xargs` features but is not constrained to the same API:
    - Argument substitution (enabled by default with `{}`)
    - Configurable input delimiter (default `\n`) including null-byte separation for inputs coming from `find -paste0`
    - Supports sentinel error codes which abort other jobs

## Usage
A job is launched with a command template if the template field is non-empty:
```
jog [options] template
```

If a template is provided, any input to the program is considered an argument.


Without a template, each input is expected to be a complete command.

### Argument substitution

Arguments are appended to the end of a template unless a replacement string is encountered, in which case arguments are substituted into that position.

By default, the replacement string is `{}` and can be modified or disabled with `--replace [string]`.

```
seq 3 | jog echo 'job_1_output'
# Runs the commands:
echo job_1_output
echo job_2_output
echo job_3_output
```

In the case of ambigious incompatible flag usage between `jog` and the command itself, you can manually declare where the template begins by using `--` as a separator:

```
jog -q -- mycmd -q
```

### Shell
By default, `jog` executes commands directly e.g. without spawning a shell process.
This means that shell functions such as expansions or redirection are not supported.
To enable shell features, use a command template such as `sh -c '{}'`.

