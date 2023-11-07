default:
	@echo "Check targets"

crep:
	$(CC) main.c \
		-I./vendor/tree-sitter/lib/include \
		-o crep \
		./vendor/tree-sitter/libtree-sitter.a \
		./vendor/tree-sitter-json/libtree-sitter-json.a \
		./vendor/tree-sitter-c/libtree-sitter-c.a \
		./vendor/tree-sitter-python/libtree-sitter-python.a

ts-build:
	-cd vendor/tree-sitter && make -B
	-cd vendor/tree-sitter-json && make -B
	-cd vendor/tree-sitter-c && make -B
	-cd vendor/tree-sitter-python && make -B

ts-clean:
	cd vendor/tree-sitter && make clean
	cd vendor/tree-sitter-json && make clean
	cd vendor/tree-sitter-c && make clean
	cd vendor/tree-sitter-python && make clean

clean:
	rm crep
