#ifndef LIST_H
#define LIST_H

typedef struct node {
  char *file_path;
  struct node *next;
} Node;

void add_file_path(Node **head, char *file_path);
void list_files_recursively(char *base_path, Node **head);
void free_file_list(Node *head);
int size_of_file_list(Node *head);

#endif
