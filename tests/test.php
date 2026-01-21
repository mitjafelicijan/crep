<?php

function simple_function() {
    echo "Hello";
}

function function_with_args($a, $b) {
    return $a + $b;
}

class MyClass {
    public function myMethod() {
        echo "Method";
    }

    protected static function protectedStaticMethod() {
        // ...
    }
}

const GLOBAL_CONST = 1;

class AnotherClass {
    const CLASS_CONST = 2;
}

interface MyInterface {
    public function interfaceMethod();
}
