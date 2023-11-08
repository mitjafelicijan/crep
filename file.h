#ifndef FILE_H
#define FILE_H

#include <stdio.h>

struct FileContent {
    const char *content;
    size_t count;
};

struct FileContent read_entire_file(const char *file_path);

#endif
