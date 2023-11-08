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
  long file_size = ftell(file);
  fseek(file, 0, SEEK_SET);

  if (file_size == -1) {
    perror("Error getting file size");
    return file_data;
  }

  file_data.content = (const char *)malloc(file_size);
  if (file_data.content == NULL) {
    perror("Error allocating memory");
    return file_data;
  }

  size_t bytes_read = fread((void *)file_data.content, 1, file_size, file);
  if (bytes_read != (size_t)file_size) {
    perror("Error reading file");
    free((void *)file_data.content);
    file_data.content = NULL;
    return file_data;
  }

  file_data.count = bytes_read;

  fclose(file);
  return file_data;
}
