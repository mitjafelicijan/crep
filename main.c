#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>

#include <tree_sitter/api.h>

#include "file.h"
#include "list.h"

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

char* remove_newlines(const char* str) {
  size_t length = strlen(str);
  char* result = (char*)malloc(length + 1);  // +1 for the null terminator
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
  const char* file_path;
  const char* source_code;
  TSLanguage* language;
  const char* cfname;
};

// void parse_source_file(const char *file_path, const char *source_code, TSLanguage *language, const char *cfname) {
void *parse_source_file(void *arg) {
  struct ThreadArgs* args = (struct ThreadArgs*)arg;

  const char *file_path = args->file_path;
  const char *source_code = args->source_code;
  TSLanguage *language = args->language;
  const char *cfname = args->cfname;

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

      // Substring matching.
      // FIXME: Add Levenshtein distance.
      char *result = strstr(fn.fname, cfname);
      if (result != NULL) {
        char *fparams_formatted = remove_newlines(fn.fparams);
        printf("%s:%zu:\t%s %s %s\n", file_path, fn.lineno, fn.ftype, fn.fname, fparams_formatted);
      }
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

  return NULL;
}

const char *get_file_extension(const char *file_path) {
  const char *extension = strrchr(file_path, '.');
  if (extension != NULL) {
    return extension + 1;
  }
  return NULL;
}

int main(int argc, char *argv[]) {
  if (argc < 3) {
    printf("Usage: %s <search term> <directory>\n", argv[0]);
    return 1;
  }

  char *cfname = argv[1];
  char *directory = argv[2];

  TSLanguage *tree_sitter_c(void);
  TSLanguage *tree_sitter_python(void);

  Node *head = NULL;
  list_files_recursively(directory, &head);
  int list_size = size_of_file_list(head);
  /* pthread_t threads[list_size]; */

  if (DEBUG) {
    printf("Scanning %d files\n", list_size);
  }

  Node *current = head;
  int thread_index = 0;
  while (current != NULL) {
    const char *file_path = current->file_path;
    const char *extension = get_file_extension(file_path);
    struct FileContent source_file = read_entire_file(file_path);

    if (source_file.content != NULL) {
      if (extension != NULL) {
        if (strcmp(extension, "c") == 0 || strcmp(extension, "h") == 0) {
          /* parse_source_file(file_path, source_file.content, tree_sitter_c(), cfname); */

          struct ThreadArgs thread_args;
          thread_args.file_path = file_path;
          thread_args.source_code = source_file.content;
          thread_args.language = tree_sitter_c();
          thread_args.cfname = cfname;

          parse_source_file(&thread_args);

          /* printf("> creating thread #%d\n", thread_index); */
          /* if (pthread_create(&threads[thread_index], NULL, parse_source_file, &thread_args) != 0) { */
          /*   fprintf(stderr, "Error creating thread %d\n", thread_index); */
          /*   return 1; */
          /* } */
        }
      }
      free((void *)source_file.content);
    } else {
      if (DEBUG) {
        fprintf(stderr, "Failed to read file.\n");
      }
    }
    current = current->next;
    thread_index++;
  }

  // Collecting threads.
  /* for (int i = 0; i < list_size; i++) { */
  /*   printf("> collecting thread #%d\n", thread_index); */
  /*   if (pthread_join(threads[i], NULL) != 0) { */
  /*     fprintf(stderr, "Error joining thread %d\n", i); */
  /*     return 1; */
  /*   } */
  /* } */

  free_file_list(head);
  return 0;
}
