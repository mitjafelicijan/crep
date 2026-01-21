#include <stdio.h>

// A simple function definition
void hello() {
    printf("Hello, world!\n");
}

// A function with parameters
int add(int a, int b) {
    return a + b;
}

// A function returning a pointer
int* get_pointer(int* x) {
    return x;
}

// A function declaration (prototype)
void declared_only(int x);

// A pointer to a function declaration
char* (*function_ptr_return)(int);

// A complex declaration
static inline const char* complex_func(const int* const ptr, void (*callback)(int)) {
    return "complex";
}

int main() {
    hello();
    return 0;
}
