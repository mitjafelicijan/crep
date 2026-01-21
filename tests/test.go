package main

import "fmt"

// Simple function
func Hello() {
	fmt.Println("Hello")
}

// Function with parameters
func Add(a int, b int) int {
	return a + b
}

// Struct type
type Point struct {
	X int
	Y int
}

// Interface type
type Describer interface {
	Describe() string
}

// Method declaration
func (p Point) Describe() string {
	return fmt.Sprintf("Point(%d, %d)", p.X, p.Y)
}

// Constant declaration
const MaxValue = 100
