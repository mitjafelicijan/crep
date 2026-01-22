const std = @import("std");

pub fn main() !void {
    std.debug.print("Hello, world!\n", .{});
}

pub fn add(a: i32, b: i32) i32 {
    return a + b;
}

const Point = struct {
    x: f32,
    y: f32,

    pub fn init(x: f32, y: f32) Point {
        return Point{ .x = x, .y = y };
    }
};

const Color = enum {
    Red,
    Green,
    Blue,
};

const MyError = error{
    BadStuff,
};
