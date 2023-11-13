#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

#include "list.h"

void add_file_path(Node** head, char* file_path) {
  Node* new = (Node*)malloc(sizeof(Node));
  new->file_path = strdup(file_path);
  new->next = *head;
  *head = new;
}

void list_files_recursively(char* base_path, Node** head) {
  char path[1000];
  struct dirent* dp;
  DIR* dir = opendir(base_path);

  if (!dir)
    return;

  while ((dp = readdir(dir)) != NULL) {
    if (strcmp(dp->d_name, ".") != 0 && strcmp(dp->d_name, "..") != 0) {
      strcpy(path, base_path);
      strcat(path, "/");
      strcat(path, dp->d_name);

      struct stat statbuf;
      if (stat(path, &statbuf) != -1) {
        if (S_ISDIR(statbuf.st_mode)) {
          list_files_recursively(path, head);
        } else {
          add_file_path(head, path);
        }
      }
    }
  }

  closedir(dir);
}

void free_file_list(Node* head) {
  Node* tmp;

  while (head != NULL) {
    tmp = head;
    head = head->next;
    free(tmp->file_path);
    free(tmp);
  }
}

int size_of_file_list(Node* head) {
  int count = 0;

  Node* current = head;
  while (current != NULL) {
    count++;
    current = current->next;
  }

  return count;
}
