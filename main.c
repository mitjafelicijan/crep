#include <stdio.h>
#include <stdlib.h>
#include <assert.h>
#include <string.h>

#include <tree_sitter/api.h>

#include "file.h"

#define DEBUG 1

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
    char *buffer = malloc(length + 1);  // +1 for the null terminator

    if (buffer != NULL) {
        snprintf(buffer, length + 1, "%.*s", (int)length, &source_code[start]);
        return buffer;
    }

    return NULL;
}

void parse_source_file(const char *file_path, const char *source_code, TSLanguage *language) {
    TSParser *parser = ts_parser_new();
    ts_parser_set_language(parser, language);

    TSTree *tree = ts_parser_parse_string(parser, NULL, source_code, strlen(source_code));
    TSNode root_node = ts_tree_root_node(tree);

    const char *query_string = "(function_definition type: (primitive_type) @ftype declarator: (function_declarator declarator: (identifier) @fname parameters: (parameter_list) @fparams))";

    uint32_t error_offset;
    TSQueryError error_type;
    TSQuery *query = ts_query_new(language, query_string, strlen(query_string), &error_offset, &error_type);

    TSQueryCursor *query_cursor = ts_query_cursor_new();
    ts_query_cursor_exec(query_cursor, query, root_node);

    if (query != NULL) {
        TSQueryMatch match;
        while (ts_query_cursor_next_match(query_cursor, &match)) {
            Function fn = {0};

            for (unsigned i = 0; i < match.capture_count; i++) {
                TSQueryCapture capture = match.captures[i];
                TSNode captured_node = capture.node;

                /* fprintf(stderr, "Query: %p, Capture index: %u\n", (void *)query, capture.index); */

                uint32_t capture_name_length;
                const char *capture_name = ts_query_capture_name_for_id(query, capture.index, &capture_name_length);

                if (strcmp(capture_name, "fname") == 0) {
                    fn.fname = extract_value(captured_node, source_code);

                    TSPoint start_point = ts_node_start_point(captured_node);
                    fn.lineno = start_point.row;
                }

                if (strcmp(capture_name, "ftype") == 0) {
                    fn.ftype = extract_value(captured_node, source_code);
                }

                if (strcmp(capture_name, "fparams") == 0) {
                    fn.fparams = extract_value(captured_node, source_code);
                }
            }

            printf("%s:%zu\t%s %s %s\n", file_path, fn.lineno, fn.ftype, fn.fname, fn.fparams);
        }
    } else {
        if (DEBUG) {
            printf("Query creation failed at offset %u with error type %d\n", error_offset, error_type);
        }
    }

    ts_query_cursor_delete(query_cursor);
    ts_query_delete(query);
    ts_tree_delete(tree);
    ts_parser_delete(parser);
}

const char *get_file_extension(const char *file_path) {
    const char *extension = strrchr(file_path, '.');
    if (extension != NULL) {
        return extension + 1;
    }
    return NULL;
}

int main(void) {
    const char *file_path = "examples/cmdline.c";
    /* const char *file_path = "examples/tabs.py"; */
    const char *extension = get_file_extension(file_path);

    TSLanguage *tree_sitter_c(void);
    TSLanguage *tree_sitter_python(void);

    struct FileContent source_file = read_entire_file(file_path);
    if (source_file.content != NULL) {
        if (DEBUG) {
            /* fprintf(stdout, "File contents:\n%s\n", source_file.content); */
            /* fprintf(stdout, "Count of characters: %zu\n", source_file.count); */
        }

        if (extension != NULL) {
            if (DEBUG) {
                fprintf(stdout, "File extension: %s\n", extension);
            }

            if (strcmp(extension, "c") == 0) {
                parse_source_file(file_path, source_file.content, tree_sitter_c());
            }

            if (strcmp(extension, "py") == 0) {
                parse_source_file(file_path, source_file.content, tree_sitter_python());
            }
        } else {
            if (DEBUG) {
                fprintf(stderr,"No file extension found.\n");
            }
        }

        free((void *)source_file.content);
    } else {
        if (DEBUG) {
            fprintf(stderr, "Failed to read file.\n");
        }
    }

    return 0;
}
