#include <stdio.h>
#include <tree_sitter/api.h>

TSLanguage *tree_sitter_c(void);
TSLanguage *tree_sitter_cpp(void);
TSLanguage *tree_sitter_python(void);
TSLanguage *tree_sitter_php(void);
TSLanguage *tree_sitter_go(void);
TSLanguage *tree_sitter_rust(void);
TSLanguage *tree_sitter_javascript(void);
TSLanguage *tree_sitter_lua(void);
TSLanguage *tree_sitter_zig(void);
TSLanguage *tree_sitter_kotlin(void);
TSLanguage *tree_sitter_odin(void);
TSLanguage *tree_sitter_tcl(void);
TSLanguage *tree_sitter_glsl(void);

int main() {
	printf("ABI version compliance check:\n");
	printf(" - C         %u\n", ts_language_version(tree_sitter_c()));
	printf(" - CPP       %u\n", ts_language_version(tree_sitter_cpp()));
	printf(" - Python    %u\n", ts_language_version(tree_sitter_python()));
	printf(" - PHP       %u\n", ts_language_version(tree_sitter_php()));
	printf(" - Go        %u\n", ts_language_version(tree_sitter_go()));
	printf(" - Rust      %u\n", ts_language_version(tree_sitter_rust()));
	printf(" - JS        %u\n", ts_language_version(tree_sitter_javascript()));
	printf(" - Lua       %u\n", ts_language_version(tree_sitter_lua()));
	printf(" - Zig       %u\n", ts_language_version(tree_sitter_zig()));
	printf(" - Kotlin    %u\n", ts_language_version(tree_sitter_kotlin()));
	printf(" - Odin      %u\n", ts_language_version(tree_sitter_odin()));
	printf(" - Tcl       %u\n", ts_language_version(tree_sitter_tcl()));
	printf(" - GLSL      %u\n", ts_language_version(tree_sitter_glsl()));
	return 0;
}
