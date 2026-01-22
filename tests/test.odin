package main

import "core:fmt"

main :: proc() {
    hello()
    fmt.println(add(10, 20))
}

hello :: proc() {
    fmt.println("Hello, Odin!")
}

add :: proc(a, b: int) -> int {
    return a + b
}

Point :: struct {
    x, y: f32,
}

Color :: enum {
    Red,
    Green,
    Blue,
}

complex_proc :: proc(name: string, age: int, flags: ..bool) {
    // ...
}
