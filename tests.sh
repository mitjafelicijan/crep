#!/bin/bash

CREP="./crep"
TEST_DIR="tests"

if [ ! -f "$CREP" ]; then
    echo "Error: crep binary not found. Please run 'make' first."
    exit 1
fi

failed=0

run_test() {
    local label=$1
    local search_term=$2
    local file=$3
    local expected_pattern=$4

    printf "Testing %-50s " "$label ($search_term)"
    output=$($CREP "$search_term" "$file")
    
    if echo "$output" | grep -q "$expected_pattern"; then
        echo "PASSED"
    else
        echo "FAILED"
        echo "  Expected pattern: $expected_pattern"
        echo "  Actual output: $output"
        failed=$((failed + 1))
    fi
}

run_test_with_flags() {
    local label=$1
    local flags=$2
    local search_term=$3
    local file=$4
    local expected_pattern=$5

    printf "Testing %-50s " "$label ($flags $search_term)"
    output=$($CREP $flags "$search_term" "$file")

    if echo "$output" | grep -q "$expected_pattern"; then
        echo "PASSED"
    else
        echo "FAILED"
        echo "  Expected pattern: $expected_pattern"
        echo "  Actual output: $output"
        failed=$((failed + 1))
    fi
}

echo "Starting tests..."
echo "----------------"

# C Tests
run_test "C Function" "hello" "$TEST_DIR/test.c" "void hello ()"
run_test "C Params" "add" "$TEST_DIR/test.c" "int add (int a, int b)"
run_test "C Pointer" "get_pointer" "$TEST_DIR/test.c" "int get_pointer (int\* x)"
run_test "C Proto" "declared_only" "$TEST_DIR/test.c" "void declared_only (int x)"

# Python Tests
run_test "Python Func" "hello" "$TEST_DIR/test.py" "def hello ()"
run_test "Python Params" "add" "$TEST_DIR/test.py" "def add (a, b)"
run_test "Python Complex" "complex_function" "$TEST_DIR/test.py" "def complex_function (a, b, c=None, \*args, \*\*kwargs)"
run_test "Python Method" "method_one" "$TEST_DIR/test.py" "def method_one (self)"

# PHP Tests
run_test "PHP Func" "simple_function" "$TEST_DIR/test.php" "function simple_function"
run_test "PHP Method" "myMethod" "$TEST_DIR/test.php" "function myMethod"
run_test "PHP Class" "MyClass" "$TEST_DIR/test.php" "class MyClass"
run_test "PHP Const" "GLOBAL_CONST" "$TEST_DIR/test.php" "GLOBAL_CONST = 1"

# Rust Tests
run_test "Rust Func" "add" "$TEST_DIR/test.rs" "fn add (a: i32, b: i32)"
run_test "Rust Struct" "Point" "$TEST_DIR/test.rs" "struct Point"
run_test "Rust Enum" "Direction" "$TEST_DIR/test.rs" "enum Direction"
run_test "Rust Trait" "Describe" "$TEST_DIR/test.rs" "trait Describe"

# Go Tests
run_test "Go Func" "Hello" "$TEST_DIR/test.go" "func Hello ()"
run_test "Go Method" "Describe" "$TEST_DIR/test.go" "func Describe (p Point)"
run_test "Go Struct" "Point" "$TEST_DIR/test.go" "type Point struct"
run_test "Go Interface" "Describer" "$TEST_DIR/test.go" "type Describer interface"
run_test "Go Const" "MaxValue" "$TEST_DIR/test.go" "const MaxValue"

# JavaScript Tests
run_test "JS Func" "hello" "$TEST_DIR/test.js" "function hello ()"
run_test "JS Params" "add" "$TEST_DIR/test.js" "function add (a, b)"
run_test "JS Class" "MyClass" "$TEST_DIR/test.js" "class MyClass"
run_test "JS Method" "myMethod" "$TEST_DIR/test.js" "myMethod (x)"
run_test "JS Object Method" "shortMethod" "$TEST_DIR/test.js" "shortMethod (a)"

# C++ Tests
run_test "C++ Func" "global_hello" "$TEST_DIR/test.cpp" "void global_hello ()"
run_test "C++ Namespace" "my_namespace" "$TEST_DIR/test.cpp" "namespace my_namespace"
run_test "C++ Class" "MyClass" "$TEST_DIR/test.cpp" "class MyClass"
run_test "C++ Struct" "MyStruct" "$TEST_DIR/test.cpp" "struct MyStruct"
run_test "C++ Method" "myMethod" "$TEST_DIR/test.cpp" "void myMethod ()"

# Lua Tests
run_test "Lua Func" "hello" "$TEST_DIR/test.lua" "function hello"
run_test "Lua Local Func" "secret_formula" "$TEST_DIR/test.lua" "function secret_formula"
run_test "Lua Assignment" "myfunc" "$TEST_DIR/test.lua" "function myfunc"
run_test "Lua Method" "greet" "$TEST_DIR/test.lua" "function MyTable:greet"
run_test "Lua Nested" "inner" "$TEST_DIR/test.lua" "function inner"

# Zig Tests
run_test "Zig Func" "main" "$TEST_DIR/test.zig" " main "
run_test "Zig Params" "add" "$TEST_DIR/test.zig" " add "
run_test "Zig Struct" "Point" "$TEST_DIR/test.zig" " Point "
run_test "Zig Enum" "Color" "$TEST_DIR/test.zig" " Color "
run_test "Zig Error" "MyError" "$TEST_DIR/test.zig" " MyError "

# Kotlin Tests
run_test "Kotlin Func" "hello" "$TEST_DIR/test.kt" "fun hello ()"
run_test "Kotlin Params" "add" "$TEST_DIR/test.kt" "fun add (a: Int, b: Int)"
run_test "Kotlin Class" "MyClass" "$TEST_DIR/test.kt" "class MyClass"
run_test "Kotlin Method" "myMethod" "$TEST_DIR/test.kt" "fun myMethod ()"
run_test "Kotlin Extension" "extensionFunc" "$TEST_DIR/test.kt" "fun extensionFunc ()"

# Odin Tests
run_test "Odin Proc" "hello" "$TEST_DIR/test.odin" "proc hello"
run_test "Odin Params" "add" "$TEST_DIR/test.odin" "proc add (a, b: int)"
run_test "Odin Struct" "Point" "$TEST_DIR/test.odin" "struct Point"
run_test "Odin Enum" "Color" "$TEST_DIR/test.odin" "enum Color"
run_test "Odin Variable Proc" "complex_proc" "$TEST_DIR/test.odin" "proc complex_proc (name: string, age: int, flags: ..bool)"

# Tcl Tests
run_test "Tcl Proc" "hello" "$TEST_DIR/test.tcl" "proc hello"
run_test "Tcl Params" "add" "$TEST_DIR/test.tcl" "proc add"
run_test "Tcl Complex" "complex_proc" "$TEST_DIR/test.tcl" "proc complex_proc"

# GLSL Tests
run_test "GLSL Func" "main" "$TEST_DIR/test.glsl" "void main"
run_test "GLSL Params" "add_vectors" "$TEST_DIR/test.glsl" "vec3 add_vectors (vec3 a, vec3 b)"
run_test "GLSL Struct" "Point" "$TEST_DIR/test.glsl" "struct Point"

# Case Sensitivity Tests
run_test "Default Case Insensitive" "foo" "tests/test.c" "void FooBar"
run_test_with_flags "Case Sensitive -c" "-c" "foobar" "tests/test.c" "void foobar"

# Depth Tests
run_test_with_flags "Depth 0 (root)" "-d 0" "level" "tests/depth_test" "void level0"
run_test_with_flags "Depth 1 (recursive)" "-d 1" "level" "tests/depth_test" "void level1"

# Levenshtein Distance Tests
run_test_with_flags "Levenshtein -l 1 match" "-l 1" "heelo" "$TEST_DIR/test.c" "void hello ()"
run_test_with_flags "Levenshtein -l 2 match" "-l 2" "heloo" "$TEST_DIR/test.c" "void hello ()"

echo "----------------"
if [ $failed -eq 0 ]; then
    echo "All tests passed!"
    exit 0
else
    echo "$failed tests failed."
    exit 1
fi
