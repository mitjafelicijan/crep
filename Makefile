TARGET = crep
SOURCES = $(wildcard *.c)
TS_ALIBS = $(shell find vendor -name "*.a" -print)
VENDOR_DIRS = $(wildcard vendor/*)
CFLAGS = $(EXTRA_FLAGS) -Wall -Wextra -std=gnu99 -pedantic -ggdb -O3
LIBS = -I./vendor/tree-sitter/lib/include

$(info VENDOR_DIRS: $(VENDOR_DIRS))
$(info SOURCES: $(SOURCES))
$(info TS_ALIBS: $(TS_ALIBS))
$(info CFLAGS: $(CFLAGS))
$(info LIBS: $(LIBS))

$(TARGET):
	$(CC) $(CFLAGS) $(SOURCES) $(LIBS) -o $(TARGET) $(TS_ALIBS)

ts-build:
	-cd vendor/tree-sitter && make -B
	-cd vendor/tree-sitter-c && make -B
	-cd vendor/tree-sitter-python && make -B
	-cd vendor/tree-sitter-json && make -B

ts-clean:
	-cd vendor/tree-sitter && make clean
	-cd vendor/tree-sitter-c && make clean
	-cd vendor/tree-sitter-python && make clean
	-cd vendor/tree-sitter-json && make clean

valgrind:
	valgrind -s --leak-check=full ./$(TARGET)

clean:
	rm -f *.o $(TARGET) callgrind.out.*
