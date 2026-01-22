// FIXME:
//  - Truncate longer argument list.

#define _GNU_SOURCE
#include <assert.h>
#include <getopt.h>
#include <pthread.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <tree_sitter/api.h>

#include "file.h"
#include "list.h"
#include "tpool.h"

#include "queries/c.h"
#include "queries/cpp.h"
#include "queries/cuda.h"
#include "queries/glsl.h"
#include "queries/go.h"
#include "queries/javascript.h"
#include "queries/kotlin.h"
#include "queries/lua.h"
#include "queries/odin.h"
#include "queries/php.h"
#include "queries/python.h"
#include "queries/rust.h"
#include "queries/tcl.h"
#include "queries/zig.h"

int debug_enabled = 0;

TSLanguage *tree_sitter_c(void);
TSLanguage *tree_sitter_cpp(void);
TSLanguage *tree_sitter_go(void);
TSLanguage *tree_sitter_python(void);
TSLanguage *tree_sitter_php(void);
TSLanguage *tree_sitter_rust(void);
TSLanguage *tree_sitter_javascript(void);
TSLanguage *tree_sitter_lua(void);
TSLanguage *tree_sitter_zig(void);
TSLanguage *tree_sitter_kotlin(void);
TSLanguage *tree_sitter_odin(void);
TSLanguage *tree_sitter_tcl(void);
TSLanguage *tree_sitter_glsl(void);
TSLanguage *tree_sitter_cuda(void);

#define MIN(a, b) ((a) < (b) ? (a) : (b))

int levenshtein_distance(const char *s1, const char *s2) {
	unsigned int len1 = strlen(s1);
	unsigned int len2 = strlen(s2);
	unsigned int distances[len1 + 1][len2 + 1];

	for (unsigned int i = 0; i <= len1; i++) {
		distances[i][0] = i;
	}
	for (unsigned int j = 0; j <= len2; j++) {
		distances[0][j] = j;
	}

	for (unsigned int i = 1; i <= len1; i++) {
		for (unsigned int j = 1; j <= len2; j++) {
			int cost = (s1[i - 1] == s2[j - 1]) ? 0 : 1;
			distances[i][j] = MIN(MIN(distances[i - 1][j] + 1, distances[i][j - 1] + 1), distances[i - 1][j - 1] + cost);
		}
	}

	return distances[len1][len2];
}

typedef struct {
	const char *fname;
	const char *ftype;
	const char *fparams;
	size_t lineno;
} Function;

const char *extract_value(TSNode captured_node, const char *source_code) {
	size_t start = ts_node_start_byte(captured_node);
	size_t end = ts_node_end_byte(captured_node);
	size_t length = end - start;
	char *buffer = malloc(length + 1); // +1 for the null terminator

	if (buffer != NULL) {
		snprintf(buffer, length + 1, "%.*s", (int)length, &source_code[start]);
		return buffer;
	} else {
		perror("malloc");
		exit(EXIT_FAILURE);
	}

	return NULL;
}

char *remove_newlines(const char *str) {
	if (str == NULL)
		return NULL;
	size_t length = strlen(str);
	char *result = (char *)malloc(length + 1); // +1 for the null terminator
	if (result == NULL) {
		fprintf(stderr, "Memory allocation failed\n");
		exit(1);
	}

	size_t j = 0;
	for (size_t i = 0; i < length; i++) {
		if (str[i] != '\n') {
			result[j++] = str[i];
		}
	}

	result[j] = '\0';
	return result;
}

struct ThreadArgs {
	const char *file_path;
	const char *source_code;
	TSLanguage *language;
	const char *query_string;
	uint32_t query_len;
	const char *cfname;
	int case_sensitive;
	int max_distance;
};

// void parse_source_file(const char *file_path, const char *source_code,
// TSLanguage *language, const char *cfname) {
void parse_source_file(void *arg) {
	struct ThreadArgs *args = (struct ThreadArgs *)arg;

	const char *file_path = args->file_path;
	const char *source_code = args->source_code;
	TSLanguage *language = args->language;
	const char *cfname = args->cfname;
	int case_sensitive = args->case_sensitive;
	int max_distance = args->max_distance;

	TSParser *parser = ts_parser_new();
	ts_parser_set_language(parser, language);

	TSTree *tree = ts_parser_parse_string(parser, NULL, source_code, strlen(source_code));
	if (tree == NULL) {
		if (debug_enabled) {
			fprintf(stderr, "Parsing failed for file: %s\n", file_path);
		}
		ts_parser_delete(parser);
		free((void *)source_code);
		free(args);
		return;
	}
	TSNode root_node = ts_tree_root_node(tree);

	const char *query_string = args->query_string;
	uint32_t query_len = args->query_len;

	uint32_t error_offset;
	TSQueryError error_type;
	TSQuery *query = ts_query_new(language, query_string, query_len, &error_offset, &error_type);

	if (query == NULL) {
		if (debug_enabled) {
			printf("Query creation failed at offset %u with error type %d\n", error_offset, error_type);
		}
		ts_tree_delete(tree);
		ts_parser_delete(parser);
		free((void *)source_code);
		free(args);
		return;
	}

	TSQueryCursor *query_cursor = ts_query_cursor_new();
	ts_query_cursor_exec(query_cursor, query, root_node);

	TSQueryMatch match;
	while (ts_query_cursor_next_match(query_cursor, &match)) {
		Function fn = {0};

		for (unsigned i = 0; i < match.capture_count; i++) {
			TSQueryCapture capture = match.captures[i];
			TSNode captured_node = capture.node;

			uint32_t capture_name_length;
			const char *capture_name = ts_query_capture_name_for_id(
				query, capture.index, &capture_name_length);

			if (strcmp(capture_name, "fname") == 0) {
				fn.fname = extract_value(captured_node, source_code);

				TSPoint start_point = ts_node_start_point(captured_node);
				fn.lineno = start_point.row + 1;
			}

			if (strcmp(capture_name, "ftype") == 0) {
				fn.ftype = extract_value(captured_node, source_code);
			}

			if (strcmp(capture_name, "fparams") == 0) {
				fn.fparams = extract_value(captured_node, source_code);
			}
		}

		// Substring matching.
		if (fn.fname != NULL) {
			char *result = NULL;
			int distance = -1;

			if (max_distance > 0) {
				distance = levenshtein_distance(fn.fname, cfname);
				if (distance <= max_distance) {
					// We treat it as a match, but result pointer logic is different
					// For printing purposes effectively a match.
					// We'll just set result to non-null to trigger the print.
					result = (char *)fn.fname;
				}
			} else {
				if (case_sensitive) {
					result = strstr(fn.fname, cfname);
				} else {
					result = strcasestr(fn.fname, cfname);
				}
			}

			if (result != NULL) {
				char *fparams_formatted = remove_newlines(fn.fparams);
				if (max_distance > 0) {
					printf("%s:%zu: %s %s %s (dist: %d)\n", file_path, fn.lineno, fn.ftype ? fn.ftype : "", fn.fname, fparams_formatted ? fparams_formatted : "", distance);
				} else {
					printf("%s:%zu: %s %s %s\n", file_path, fn.lineno, fn.ftype ? fn.ftype : "", fn.fname, fparams_formatted ? fparams_formatted : "");
				}
				free(fparams_formatted);
			}
		}

		// Free captured values
		free((void *)fn.fname);
		free((void *)fn.ftype);
		free((void *)fn.fparams);
	}

	ts_query_cursor_delete(query_cursor);
	ts_query_delete(query);
	ts_tree_delete(tree);
	ts_parser_delete(parser);

	// Cleanup thread arguments
	free((void *)source_code);
	free(args);
}

const char *get_file_extension(const char *file_path) {
	const char *extension = strrchr(file_path, '.');
	if (extension != NULL) {
		return extension + 1;
	}
	return NULL;
}

int main(int argc, char *argv[]) {
	int case_sensitive = 0;
	int max_distance = 0;
	int max_depth = -1;
	int opt;
	struct option long_options[] = {
		{"case-sensitive", no_argument, 0, 'c'},
		{"levenshtein", required_argument, 0, 'l'},
		{"depth", required_argument, 0, 'd'},
		{0, 0, 0, 0}};

	while ((opt = getopt_long(argc, argv, "cl:d:", long_options, NULL)) != -1) {
		switch (opt) {
		case 'c':
			case_sensitive = 1;
			break;
		case 'l':
			max_distance = atoi(optarg);
			break;
		case 'd':
			max_depth = atoi(optarg);
			break;
		default:
			fprintf(stderr, "Usage: %s [-c|--case-sensitive] [-l|--levenshtein <dist>] [-d|--depth <level>] <search term> [directory|file]\n", argv[0]);
			return 1;
		}
	}

	if (optind >= argc) {
		fprintf(stderr, "Usage: %s [-c|--case-sensitive] [-l|--levenshtein <dist>] [-d|--depth <level>] <search term> [directory|file]\n", argv[0]);
		return 1;
	}

	const char *cfname = argv[optind];
	char *directory = (optind + 1 < argc) ? argv[optind + 1] : ".";

	Node *head = NULL;
	list_files_recursively(directory, &head, max_depth, 0);
	int list_size = size_of_file_list(head);

	const char *debug_env = getenv("DEBUG");
	if (debug_env != NULL && (strcmp(debug_env, "1") == 0 || strcmp(debug_env, "true") == 0)) {
		debug_enabled = 1;
	}

	if (debug_enabled) {
		printf("Scanning %d files\n", list_size);
	}

	ThreadPool *pool = tp_create(8);
	if (!pool) {
		perror("Failed to create thread pool");
		return 1;
	}

	Node *current = head;
	while (current != NULL) {
		const char *file_path = current->file_path;
		const char *extension = get_file_extension(file_path);

		TSLanguage *lang = NULL;
		const char *query_string = NULL;
		uint32_t query_len = 0;

		if (extension != NULL) {
			if (strcmp(extension, "c") == 0 || strcmp(extension, "h") == 0) {
				lang = tree_sitter_c();
				query_string = (const char *)query_c;
				query_len = query_c_len;
			} else if (strcmp(extension, "cpp") == 0 || strcmp(extension, "hpp") == 0) {
				lang = tree_sitter_cpp();
				query_string = (const char *)query_cpp;
				query_len = query_cpp_len;
			} else if (strcmp(extension, "go") == 0) {
				lang = tree_sitter_go();
				query_string = (const char *)query_go;
				query_len = query_go_len;
			} else if (strcmp(extension, "py") == 0) {
				lang = tree_sitter_python();
				query_string = (const char *)query_python;
				query_len = query_python_len;
			} else if (strcmp(extension, "php") == 0) {
				lang = tree_sitter_php();
				query_string = (const char *)query_php;
				query_len = query_php_len;
			} else if (strcmp(extension, "rs") == 0) {
				lang = tree_sitter_rust();
				query_string = (const char *)query_rust;
				query_len = query_rust_len;
			} else if (strcmp(extension, "js") == 0) {
				lang = tree_sitter_javascript();
				query_string = (const char *)query_javascript;
				query_len = query_javascript_len;
			} else if (strcmp(extension, "lua") == 0) {
				lang = tree_sitter_lua();
				query_string = (const char *)query_lua;
				query_len = query_lua_len;
			} else if (strcmp(extension, "zig") == 0) {
				lang = tree_sitter_zig();
				query_string = (const char *)query_zig;
				query_len = query_zig_len;
			} else if (strcmp(extension, "kt") == 0) {
				lang = tree_sitter_kotlin();
				query_string = (const char *)query_kotlin;
				query_len = query_kotlin_len;
			} else if (strcmp(extension, "odin") == 0) {
				lang = tree_sitter_odin();
				query_string = (const char *)query_odin;
				query_len = query_odin_len;
			} else if (strcmp(extension, "tcl") == 0) {
				lang = tree_sitter_tcl();
				query_string = (const char *)query_tcl;
				query_len = query_tcl_len;
			} else if (strcmp(extension, "glsl") == 0) {
				lang = tree_sitter_glsl();
				query_string = (const char *)query_glsl;
				query_len = query_glsl_len;
			} else if (strcmp(extension, "cu") == 0 || strcmp(extension, "cuh") == 0) {
				lang = tree_sitter_cuda();
				query_string = (const char *)query_cuda;
				query_len = query_cuda_len;
			}
		}

		if (lang != NULL && query_string != NULL) {
			struct FileContent source_file = read_entire_file(file_path);
			if (source_file.content != NULL) {
				struct ThreadArgs *thread_args = malloc(sizeof(struct ThreadArgs));
				if (!thread_args) {
					perror("Failed to allocate thread args");
					free((void *)source_file.content);
					continue;
				}

				thread_args->file_path = file_path;
				thread_args->source_code = source_file.content;
				thread_args->language = lang;
				thread_args->query_string = query_string;
				thread_args->query_len = query_len;
				thread_args->cfname = cfname;
				thread_args->case_sensitive = case_sensitive;
				thread_args->max_distance = max_distance;

				tp_add_job(pool, (thread_func_t)parse_source_file, thread_args);
			} else {
				if (debug_enabled) {
					fprintf(stderr, "Failed to read file: %s\n", file_path);
				}
			}
		}

		current = current->next;
	}

	tp_wait(pool);
	tp_destroy(pool);
	free_file_list(head);
	return 0;
}
