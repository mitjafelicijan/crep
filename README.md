`crep` is a command-line tool designed for searching code using
[Tree-sitter](https://tree-sitter.github.io/tree-sitter/) queries. Unlike
traditional `grep`, which operates on text lines, `crep` understands the
structure of your code, allowing for more precise semantic searching.

> [!IMPORTANT]
> `crep` was built for fun and is not intended for real use. I made it to learn
> Tree-sitter and to have fun.

## Features

- **Semantic Search**: Uses Tree-sitter to parse code into Concrete Syntax Trees
  (CSTs) and execute queries against them.
- **Broad Language Support**: Supports a wide range of languages including C, C++,
  Rust, Python, Go, and others (see full list below).
- **Multi-threaded**: Utilizes a custom thread pool for efficient scanning of
  large codebases.
- **Structural Matching**: Reports file path, line number, return type, function
  name, and parameters for each match.
- **Debug Mode**: Supports detailed logging via the `DEBUG` environment
  variable.
- **Fuzzy Matching**: Supports configurable Levenshtein distance for fuzzy search.

## Prerequisites

To build `crep`, you need:
- A C compiler (GCC or Clang)
- `make`
- `xxd` (used for embedding Tree-sitter queries)
- `pthread` library

## Installation

```bash
git clone https://github.com/mitjafelicijan/crep.git
cd crep
make all
```

## Usage

```bash
./crep [-c|--case-sensitive] [-l|--levenshtein <dist>] [-d|--depth <level>] <search_term> [path]
```

- `-c, --case-sensitive`: Enable case-sensitive matching (default is case-insensitive).
- `-l, --levenshtein <dist>`: Enable fuzzy matching with a maximum Levenshtein distance of `<dist>`.
- `-d, --depth <level>`: Set the maximum recursion depth for directory traversal.
- `<search_term>`: The string to search for within function/method names.
- `[path]`: Optional. The directory or file to search (defaults to current directory).

### Environment Variables

- `DEBUG=1` or `DEBUG=true`: Enable verbose debug logging.

### Examples

Search for all functions containing "init" in the current directory:

```bash
./crep init .
```

Search for functions containing "parse" in a specific file:
```bash
./crep parse main.c
```

Run with debug logging enabled:
```bash
DEBUG=1 ./crep init .
```

Search for "main" allowing for 2 typos (e.g. "mian"):

```bash
./crep -l 2 "mian" main.c
```

## How It Works

`crep` works by:

1. Identifying supported files based on extension.
2. Parsing the files using language-specific Tree-sitter grammars.
3. Executing a structural query to find function/method definitions, including
   return types and parameters.
4. Matching the identifier against the provided search term.
5. Displaying the results with added semantic context.

## Supported Languages

| Language   | Extensions     |
| ---------- | -------------- |
| C          | `.c`, `.h`     |
| C++        | `.cpp`, `.hpp` |
| CUDA       | `.cu`, `.cuh`  |
| GLSL       | `.glsl`        |
| Go         | `.go`          |
| JavaScript | `.js`          |
| Kotlin     | `.kt`          |
| Lua        | `.lua`         |
| Odin       | `.odin`        |
| PHP        | `.php`         |
| Python     | `.py`          |
| Rust       | `.rs`          |
| Tcl        | `.tcl`         |
| Zig        | `.zig`         |

## Running Tests

To run the test suite:

```bash
make tests
```

## Additional resources

- https://en.wikipedia.org/wiki/Ctags
- https://www.emacswiki.org/emacs/TagsFile
- https://en.wikipedia.org/wiki/Boyer%E2%80%93Moore_string-search_algorithm
- https://en.wikipedia.org/wiki/Levenshtein_distance
- https://github.com/tree-sitter/tree-sitter
- https://tree-sitter.github.io/tree-sitter/playground
- https://dreampuf.github.io/GraphvizOnline/
- https://github.com/tree-sitter-grammars
