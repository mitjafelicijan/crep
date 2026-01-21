`crep` is a command-line tool designed for searching code using [Tree-sitter](https://tree-sitter.github.io/tree-sitter/) queries. Unlike traditional `grep`, which operates on text lines, `crep` understands the structure of your code, allowing for more precise semantic searching.

## Features

- **Semantic Search**: Uses Tree-sitter to parse code into Concrete Syntax Trees (CSTs) and execute queries against them.
- **Language Support**: Currently supports **C** and **Python**.
- **Multi-threaded**: Utilizes a custom thread pool for efficient scanning of large codebases.
- **Detailed Output**: Reports file path, line number, return type, function name, and parameters for each match.

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

- `<search_term>`: The string to search for within function names.
- `[path]`: Optional. The directory or file to search (defaults to current directory).

### Examples

Search for all functions containing "init" in the current directory:
```bash
./crep init .
```

Search for functions containing "parse" in a specific file:
```bash
./crep parse main.c
```

## How It Works

`crep` works by:
1. Identifying supported files.
2. Parsing the files using Tree-sitter grammars.
3. Executing a structural query to find function/method definitions.
4. Matching the identifier against the provided search term.
5. Displaying the results in a format similar to grep but with added semantic context.

## Additional resources

- https://en.wikipedia.org/wiki/Ctags
- https://www.emacswiki.org/emacs/TagsFile
- https://en.wikipedia.org/wiki/Boyer%E2%80%93Moore_string-search_algorithm
- https://en.wikipedia.org/wiki/Levenshtein_distance
- https://github.com/tree-sitter/tree-sitter
- https://tree-sitter.github.io/tree-sitter/playground
- https://dreampuf.github.io/GraphvizOnline/
