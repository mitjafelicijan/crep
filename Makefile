.PHONY: all query ts-build ts-clean valgrind tests format clean

TARGET = crep
SOURCES = $(wildcard *.c *.h)
TS_ALIBS = $(shell find vendor -name "*.a" -print)
VENDOR_DIRS = $(wildcard vendor/*)
CFLAGS = $(EXTRA_FLAGS) -Wall -Wextra -std=gnu99 -pedantic -O3
LIBS = -I./vendor/tree-sitter/lib/include -lpthread

$(info VENDOR_DIRS: $(VENDOR_DIRS))
$(info SOURCES: $(SOURCES))
$(info TS_ALIBS: $(TS_ALIBS))
$(info CFLAGS: $(CFLAGS))
$(info LIBS: $(LIBS))

$(TARGET):
	$(CC) $(CFLAGS) $(SOURCES) $(LIBS) -o $(TARGET) $(TS_ALIBS)

all: ts-build query $(TARGET)

query:
	xxd -i -n query_c queries/c.scm > queries/c.h
	xxd -i -n query_python queries/python.scm > queries/python.h
	xxd -i -n query_php queries/php.scm > queries/php.h
	xxd -i -n query_go queries/go.scm > queries/go.h
	xxd -i -n query_rust queries/rust.scm > queries/rust.h

ts-build:
	-cd vendor/tree-sitter && make -B
	-cd vendor/tree-sitter-c && make -B
	-cd vendor/tree-sitter-python && make -B
	-cd vendor/tree-sitter-php && make -B
	-cd vendor/tree-sitter-go && make -B
	-cd vendor/tree-sitter-rust && make -B

ts-clean:
	-cd vendor/tree-sitter && make clean
	-cd vendor/tree-sitter-c && make clean
	-cd vendor/tree-sitter-python && make clean
	-cd vendor/tree-sitter-php && make clean
	-cd vendor/tree-sitter-go && make clean
	-cd vendor/tree-sitter-rust && make clean

valgrind:
	valgrind -s --leak-check=full ./$(TARGET)

tests: $(TARGET)
	sh tests.sh

format:
	clang-format -i *.c *.h

clean:
	rm -f *.o $(TARGET) callgrind.out.*
