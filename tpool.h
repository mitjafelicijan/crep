#ifndef THREAD_POOL_H
#define THREAD_POOL_H

#include <pthread.h>
#include <stdbool.h>

typedef void (*thread_func_t)(void *arg);

typedef struct {
	thread_func_t function;
	void *arg;
} ThreadPoolJob;

typedef struct ThreadPool ThreadPool;

ThreadPool *tp_create(int num_threads);
void tp_add_job(ThreadPool *pool, thread_func_t function, void *arg);
void tp_wait(ThreadPool *pool);
void tp_destroy(ThreadPool *pool);

#endif
