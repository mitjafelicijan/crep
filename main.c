// TODO:
//  - Add language specific filter (by default all but it can also passed
//    with -tpy -tc -trb) which would only parse python, c and ruby files.
//  - By default its case insensitive but with passing -cs it tells that
//    all matching should be done in case sensitive way.
//  - Add Levenshtein distance for matching and expose distance as arg with
//    something like -d5 which would allow distance of 5 on a match.
//  - Allow DEBUG to be provided as environmental variable.
//  - Added depth flag (-r means recursive, -l2 means 2 levels deep).

// FIXME:
//  - Truncate longer argument list.

#include <assert.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <tree_sitter/api.h>

#include "file.h"
#include "list.h"
#include "tpool.h"

#include "queries/c.h"
#include "queries/go.h"
#include "queries/php.h"
#include "queries/python.h"
#include "queries/rust.h"
#include "queries/javascript.h"

int debug_enabled = 0;

TSLanguage *tree_sitter_c(void);
TSLanguage *tree_sitter_go(void);
TSLanguage *tree_sitter_python(void);
TSLanguage *tree_sitter_php(void);
TSLanguage *tree_sitter_rust(void);
TSLanguage *tree_sitter_javascript(void);

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
};

// void parse_source_file(const char *file_path, const char *source_code,
// TSLanguage *language, const char *cfname) {
void parse_source_file(void *arg) {
	struct ThreadArgs *args = (struct ThreadArgs *)arg;

	const char *file_path = args->file_path;
	const char *source_code = args->source_code;
	TSLanguage *language = args->language;
	const char *cfname = args->cfname;

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
		// FIXME: Add Levenshtein distance.
		if (fn.fname != NULL) {
			char *result = strstr(fn.fname, cfname);
			if (result != NULL) {
				char *fparams_formatted = remove_newlines(fn.fparams);
				printf("%s:%zu: %s %s %s\n", file_path, fn.lineno, fn.ftype ? fn.ftype : "", fn.fname, fparams_formatted ? fparams_formatted : "");
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
	if (argc < 2) {
		printf("Usage: %s <search term> [directory|file]\n", argv[0]);
		return 1;
	}

	const char *cfname = argv[1];
	char *directory = (argc > 2) ? argv[2] : ".";

	Node *head = NULL;
	list_files_recursively(directory, &head);
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
