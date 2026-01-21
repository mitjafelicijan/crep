#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

#include "list.h"

void add_file_path(Node **head, char *file_path) {
	Node *new = (Node *)malloc(sizeof(Node));
	if (new == NULL) {
		perror("malloc");
		exit(EXIT_FAILURE);
	}
	new->file_path = strdup(file_path);
	if (new->file_path == NULL) {
		perror("strdup");
		free(new);
		exit(EXIT_FAILURE);
	}
	new->next = *head;
	*head = new;
}

void list_files_recursively(char *base_path, Node **head) {
	struct stat statbuf;
	if (stat(base_path, &statbuf) == -1) {
		perror("stat");
		return;
	}

	if (S_ISREG(statbuf.st_mode)) {
		add_file_path(head, base_path);
		return;
	}

	char path[2048];
	struct dirent *dp;
	DIR *dir = opendir(base_path);

	if (!dir)
		return;

	while ((dp = readdir(dir)) != NULL) {
		if (strcmp(dp->d_name, ".") != 0 && strcmp(dp->d_name, "..") != 0) {
			int ret = snprintf(path, sizeof(path), "%s%s%s", base_path, (base_path[strlen(base_path) - 1] == '/' ? "" : "/"), dp->d_name);

			if (ret >= (int)sizeof(path)) {
				fprintf(stderr, "Path too long: %s/%s\n", base_path, dp->d_name);
				continue;
			}

			if (stat(path, &statbuf) != -1) {
				if (S_ISDIR(statbuf.st_mode)) {
					list_files_recursively(path, head);
				} else if (S_ISREG(statbuf.st_mode)) {
					add_file_path(head, path);
				}
			}
		}
	}

	closedir(dir);
}

void free_file_list(Node *head) {
	Node *tmp;

	while (head != NULL) {
		tmp = head;
		head = head->next;
		free(tmp->file_path);
		free(tmp);
	}
}

int size_of_file_list(Node *head) {
	int count = 0;

	Node *current = head;
	while (current != NULL) {
		count++;
		current = current->next;
	}

	return count;
}
