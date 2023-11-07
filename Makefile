default:
	@echo "Check targets"

crep:
	$(CC) main.c \
		-I./vendor/tree-sitter/lib/include \
		-o crep \
		./vendor/tree-sitter/libtree-sitter.a \
		./vendor/tree-sitter-json/libtree-sitter-crep.a

clean:
	rm crep
