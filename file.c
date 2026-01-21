#include <stdio.h>
#include <stdlib.h>

#include "file.h"

struct FileContent read_entire_file(const char *file_path) {
	struct FileContent file_data;
	file_data.content = NULL;
	file_data.count = 0;

	FILE *file = fopen(file_path, "rb");
	if (file == NULL) {
		perror("Error opening file");
		return file_data;
	}

	fseek(file, 0, SEEK_END);
	long raw_size = ftell(file);
	fseek(file, 0, SEEK_SET);

	if (raw_size == -1) {
		perror("Error getting file size");
		fclose(file);
		return file_data;
	}

	size_t file_size = (size_t)raw_size;
	char *content = (char *)malloc(file_size + 1);
	if (content == NULL) {
		perror("Error allocating memory");
		fclose(file);
		return file_data;
	}

	size_t bytes_read = fread(content, 1, file_size, file);
	if (bytes_read != file_size) {
		perror("Error reading file");
		free(content);
		fclose(file);
		return file_data;
	}

	content[file_size] = '\0';
	file_data.content = content;
	file_data.count = file_size;

	fclose(file);
	return file_data;
}
