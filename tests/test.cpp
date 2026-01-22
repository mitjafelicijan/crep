#include <iostream>
#include <string>

namespace my_namespace {
    void namespaced_function() {
        std::cout << "Namespaced function" << std::endl;
    }

    class MyClass {
    public:
        void myMethod() {
            std::cout << "Method inside class" << std::endl;
        }
    };
}

struct MyStruct {
    int x;
    void structMethod() {
        std::cout << "Method inside struct" << std::endl;
    }
};

void global_hello() {
    std::cout << "Global hello" << std::endl;
}

int main() {
    global_hello();
    my_namespace::namespaced_function();
    return 0;
}
