![logo](logo/logo.svg)

A simple and powerful task runner for concurrent command execution.

Aims to do _less_ than other task launchers, and stays out of your way while scaling up to millions of tasks.
You can think of it like a cousin of `parallel` or `xargs` but for green threads using goroutines.


`jog` applies simple controls for *concurrency* and *retry strategy* to any program invocation.
It plays nicely with Linux utilities such as `find`, and is designed to have a large number of arguments streamed into it.
A common use case is to apply a program to a large directory of input files without needing to write methods for batch processing within the application code.

## Features
- Run commands concurrently with no changes to application code. This allow processes with downtime (think API calls or database queries) to efficiently share CPU time.
- Reasonable options for handling failures (ignore, retry with backoff, abort, etc.).
- No YAML, DSL, DAG, functions, or recipes, and no textbook required. There are good workflow orchestrators out there (and bad ones, too). `jog` just runs your commands.
- Optional structured logging.
- Minimal execution footprint. Want to run a few million of something? Go for it!

## Usage
A job is launched with a command template if the template field is non-empty:
```
jog [options] template
```
If a template is provided, any input to the program is considered an argument. Without a template, each input is expected to be a complete command.

### Options
    -attempts uint
            maximum number of attempts per task (default 1) (default 1)
    -debug
            include debug output
    -delim string
            command delimiter (default "\n")
    -dir string
            execute commands from the given directory (default .)
    -dryrun
            print all planned commands but do not execute
    -exit
            cancel all jobs upon any job reaching an error
    -file string
            read command file for items instead of stdin
    -jobs uint
            maximum number of concurrent jobs to run (default 0 = unlimited)
    -log
            print structured logs for all processes to stderr
    -max-delay uint
            maximum wait time (ms) between attempts
    -min-delay uint
            minimum wait time (ms) between attempts
    -null
            use null byte as the delimiter (overwrites -delim)
    -progress
            display progress bar to stdout
    -replace string
            replace instances of str with args read from stdin. Default {}, disable by setting to an empty string (default "{}")

### Argument substitution
Arguments are appended to the end of a template unless a replacement string is encountered, in which case arguments are substituted into that position. 

By default, the replacement string is `{}` and can be modified or disabled with `--replace [string]`.

When constructing a template, `--dryrun` can be used to preview the resulting commands:
```
$ seq 3 | jog --dryrun echo 'job_{}_output'
echo job_1_output
echo job_2_output
echo job_3_output
```

In the case of ambigious or incompatible flag usage between `jog` and the command itself, or purely to improve readibility, you may declare where the template begins using `--` as a separator:

```
jog --file arg_file.txt -- mycmd --arg {} --file data_file.txt
```

### Shell
`jog` executes commands without spawning a shell process.
This means that shell features such as expansions or redirection are not automatically available.
To enable shell functionality, use a command template such as `sh -c '{}'`.

```
seq 3 | ./jog sh -c "'echo {} >> out_{}.txt'"
```

## Why?
In bioinformatics it is common to use a variety of specialized scientific software, often originating from small communities or academic labs.
This melange of software spans a variety of languages and (often lackluster) support for running at scale.
`jog` is a compromise which provides much of the benefit of proper concurrency with no need to dive into program internals.

Note that for CPU-limited tasks, GNU `parallel` is an adequate solution, and can trivially saturate threads with work to be done.
`jog` is meant to fill a specific niche around green threading, when running more jobs than threads is feasible but requires additional coordination, especially around jobs which hit remote APIs and must consider rate limiting and retries.
