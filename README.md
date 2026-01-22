`crep` is a command-line tool designed for searching code using
[Tree-sitter](https://tree-sitter.github.io/tree-sitter/) queries. Unlike
traditional `grep`, which operates on text lines, `crep` understands the
structure of your code, allowing for more precise semantic searching.

## Features

- **Semantic Search**: Uses Tree-sitter to parse code into Concrete Syntax Trees
  (CSTs) and execute queries against them.
- **Broad Language Support**: Supports multiple languages including C, C++, Go,
  Python, PHP, Rust, and JavaScript.
- **Multi-threaded**: Utilizes a custom thread pool for efficient scanning of
  large codebases.
- **Structural Matching**: Reports file path, line number, return type, function
  name, and parameters for each match.
- **Debug Mode**: Supports detailed logging via the `DEBUG` environment
  variable.

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
./crep <search_term> [path]
```

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
| Go         | `.go`          |
| Python     | `.py`          |
| PHP        | `.php`         |
| Rust       | `.rs`          |
| JavaScript | `.js`          |

## Additional resources

- https://en.wikipedia.org/wiki/Ctags
- https://www.emacswiki.org/emacs/TagsFile
- https://en.wikipedia.org/wiki/Boyer%E2%80%93Moore_string-search_algorithm
- https://en.wikipedia.org/wiki/Levenshtein_distance
- https://github.com/tree-sitter/tree-sitter
- https://tree-sitter.github.io/tree-sitter/playground
- https://dreampuf.github.io/GraphvizOnline/
