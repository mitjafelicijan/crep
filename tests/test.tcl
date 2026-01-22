proc hello {} {
    puts "Hello, World!"
}

proc add {a b} {
    return [expr {$a + $b}]
}

proc complex_proc {name {age 0} args} {
    puts "Name: $name, Age: $age, Args: $args"
}
