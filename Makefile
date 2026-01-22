.PHONY: all query tsbuild valgrind tests format clean

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

all: query tsbuild $(TARGET)

query:
	xxd -i -n query_c queries/c.scm > queries/c.h
	xxd -i -n query_python queries/python.scm > queries/python.h
	xxd -i -n query_php queries/php.scm > queries/php.h
	xxd -i -n query_go queries/go.scm > queries/go.h
	xxd -i -n query_rust queries/rust.scm > queries/rust.h
	xxd -i -n query_javascript queries/javascript.scm > queries/javascript.h

tsbuild:
	-$(MAKE) -C vendor/tree-sitter -B
	-$(MAKE) -C vendor/tree-sitter-c -B
	-$(MAKE) -C vendor/tree-sitter-python -B
	-$(MAKE) -C vendor/tree-sitter-php -B
	-$(MAKE) -C vendor/tree-sitter-go -B
	-$(MAKE) -C vendor/tree-sitter-rust -B
	-$(MAKE) -C vendor/tree-sitter-javascript -B

valgrind:
	valgrind -s --leak-check=full ./$(TARGET)

tests: $(TARGET)
	sh tests.sh

format:
	clang-format -i *.c *.h

clean:
	rm -f *.o $(TARGET) callgrind.out.*
	$(MAKE) -C vendor/tree-sitter -B clean
	$(MAKE) -C vendor/tree-sitter-c -B clean
	$(MAKE) -C vendor/tree-sitter-python -B clean
	$(MAKE) -C vendor/tree-sitter-php -B clean
	$(MAKE) -C vendor/tree-sitter-go -B clean
	$(MAKE) -C vendor/tree-sitter-rust -B clean
	$(MAKE) -C vendor/tree-sitter-javascript -B clean
