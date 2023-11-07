#include <stdio.h>
#include <assert.h>
#include <string.h>
#include <tree_sitter/api.h>

TSLanguage *tree_sitter_json();

int main() {
  printf("Hi, Mark\n");
  
  TSParser *parser = ts_parser_new();
  ts_parser_set_language(parser, tree_sitter_json());

  const char *source_code = "[1, null, [1,2,3]]";
  TSTree *tree = ts_parser_parse_string(
    parser,
    NULL,
    source_code,
    strlen(source_code)
  );

  TSNode root_node = ts_tree_root_node(tree);

  TSNode array_node = ts_node_named_child(root_node, 0);
  TSNode number_node = ts_node_named_child(array_node, 0);

  assert(strcmp(ts_node_type(root_node), "document") == 0);
  assert(strcmp(ts_node_type(array_node), "array") == 0);
  assert(strcmp(ts_node_type(number_node), "number") == 0);

  /* assert(ts_node_child_count(root_node) == 1); */
  /* assert(ts_node_child_count(array_node) == 5); */
  /* assert(ts_node_named_child_count(array_node) == 2); */
  /* assert(ts_node_child_count(number_node) == 0); */

  char *string = ts_node_string(root_node);
  printf("Syntax tree: %s\n", string);

  free(string);
  ts_tree_delete(tree);
  ts_parser_delete(parser);
  
  return 0;
}
