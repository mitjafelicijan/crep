-- Standard function declaration
function hello(name)
    print("Hello, " .. name)
end

-- Local function declaration
local function secret_formula(x, y)
    return x * y + 42
end

-- Function assigned to a variable
myfunc = function(a, b)
    return a - b
end

-- Method-like function
local MyTable = {}
function MyTable:greet()
    print("Greetings!")
end

-- Nested function
function outer()
    local function inner()
        print("I am inside")
    end
    inner()
end

hello("User")
secret_formula(1, 2)
myfunc(10, 5)
MyTable:greet()
outer()
