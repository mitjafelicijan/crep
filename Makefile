.PHONY: all queries tsbuild valgrind tests format clean

TARGET = crep
ABI_CHECK_TARGET = abicheck
SOURCES = $(filter-out check.c abicheck.c, $(wildcard *.c *.h queries/*.h))
TS_ALIBS = $(shell find vendor -name "*.a" -print)
VENDOR_DIRS = $(wildcard vendor/*)
CFLAGS = $(EXTRA_FLAGS) -Wall -Wextra -std=gnu99 -pedantic -O3
LIBS = -I./vendor/tree-sitter/lib/include -lpthread

LANGS = c cpp python php go rust javascript lua zig kotlin odin tcl glsl
QUERY_HEADERS = $(patsubst %, queries/%.h, $(LANGS))
TS_SUBDIRS = tree-sitter $(patsubst %, tree-sitter-%, $(LANGS))

$(info VENDOR_DIRS: $(VENDOR_DIRS))
$(info LANGS: $(LANGS))
$(info QUERY_HEADERS: $(QUERY_HEADERS))
$(info TS_SUBDIRS: $(TS_SUBDIRS))
$(info SOURCES: $(SOURCES))
$(info TS_ALIBS: $(TS_ALIBS))
$(info CFLAGS: $(CFLAGS))
$(info LIBS: $(LIBS))

all: $(QUERY_HEADERS) tsbuild $(TARGET) $(ABI_CHECK_TARGET)

tsbuild:
	$(MAKE) -C vendor/tree-sitter libtree-sitter.a
	@for lang in $(LANGS); do \
		$(MAKE) -C vendor/tree-sitter-$$lang libtree-sitter-$$lang.a || true; \
	done

$(TARGET): $(SOURCES) tsbuild
	$(CC) $(CFLAGS) $(SOURCES) $(LIBS) -o $(TARGET) $(shell find vendor -name "*.a")

$(ABI_CHECK_TARGET): abicheck.c tsbuild
	$(CC) $(CFLAGS) abicheck.c $(LIBS) $(filter-out %/libtree-sitter.a, $(wildcard vendor/**/*.a)) vendor/tree-sitter/libtree-sitter.a -o $(ABI_CHECK_TARGET)

queries/%.h: queries/%.scm
	xxd -i -n query_$* $< > $@

queries: $(QUERY_HEADERS)

valgrind:
	valgrind -s --leak-check=full ./$(TARGET)

tests: $(TARGET)
	sh tests.sh

format:
	clang-format -i *.c *.h

clean:
	rm -f *.o $(TARGET) $(ABI_CHECK_TARGET) callgrind.out.* queries/*.h
	@for dir in $(TS_SUBDIRS); do \
		$(MAKE) -C vendor/$$dir clean; \
	done
