#include "tpool.h"
#include <stdio.h>
#include <stdlib.h>

typedef struct ThreadPoolJobNode {
	ThreadPoolJob job;
	struct ThreadPoolJobNode *next;
} ThreadPoolJobNode;

struct ThreadPool {
	pthread_mutex_t lock;
	pthread_cond_t notify;
	pthread_cond_t working_cond;

	pthread_t *threads;
	int num_threads;

	ThreadPoolJobNode *queue_head;
	ThreadPoolJobNode *queue_tail;

	int active_jobs; // Jobs currently running
	int queued_jobs; // Jobs waiting in queue
	bool stop;
};

static void *tp_worker(void *arg) {
	ThreadPool *pool = (ThreadPool *)arg;

	while (1) {
		pthread_mutex_lock(&pool->lock);

		while (pool->queue_head == NULL && !pool->stop) {
			pthread_cond_wait(&pool->notify, &pool->lock);
		}

		if (pool->stop && pool->queue_head == NULL) {
			pthread_mutex_unlock(&pool->lock);
			break;
		}

		ThreadPoolJobNode *node = pool->queue_head;
		pool->queue_head = node->next;
		if (pool->queue_head == NULL) {
			pool->queue_tail = NULL;
		}

		pool->queued_jobs--;
		pool->active_jobs++;

		pthread_mutex_unlock(&pool->lock);

		// Execute job
		if (node->job.function) {
			node->job.function(node->job.arg);
		}
		free(node);

		pthread_mutex_lock(&pool->lock);
		pool->active_jobs--;
		if (pool->active_jobs == 0 && pool->queue_head == NULL) {
			pthread_cond_signal(&pool->working_cond);
		}
		pthread_mutex_unlock(&pool->lock);
	}

	return NULL;
}

ThreadPool *tp_create(int num_threads) {
	ThreadPool *pool = (ThreadPool *)malloc(sizeof(ThreadPool));
	if (pool == NULL)
		return NULL;

	pool->num_threads = num_threads;
	pool->queue_head = NULL;
	pool->queue_tail = NULL;
	pool->active_jobs = 0;
	pool->queued_jobs = 0;
	pool->stop = false;

	pthread_mutex_init(&pool->lock, NULL);
	pthread_cond_init(&pool->notify, NULL);
	pthread_cond_init(&pool->working_cond, NULL);

	pool->threads = (pthread_t *)malloc(sizeof(pthread_t) * num_threads);
	for (int i = 0; i < num_threads; i++) {
		pthread_create(&pool->threads[i], NULL, tp_worker, pool);
	}

	return pool;
}

void tp_add_job(ThreadPool *pool, thread_func_t function, void *arg) {
	ThreadPoolJobNode *node = (ThreadPoolJobNode *)malloc(sizeof(ThreadPoolJobNode));
	if (node == NULL) {
		perror("malloc");
		exit(EXIT_FAILURE);
	}
	node->job.function = function;
	node->job.arg = arg;
	node->next = NULL;

	pthread_mutex_lock(&pool->lock);

	if (pool->queue_tail) {
		pool->queue_tail->next = node;
	} else {
		pool->queue_head = node;
	}
	pool->queue_tail = node;

	pool->queued_jobs++;
	pthread_cond_signal(&pool->notify);

	pthread_mutex_unlock(&pool->lock);
}

void tp_wait(ThreadPool *pool) {
	pthread_mutex_lock(&pool->lock);
	while (pool->active_jobs > 0 || pool->queue_head != NULL) {
		pthread_cond_wait(&pool->working_cond, &pool->lock);
	}
	pthread_mutex_unlock(&pool->lock);
}

void tp_destroy(ThreadPool *pool) {
	pthread_mutex_lock(&pool->lock);
	pool->stop = true;
	pthread_cond_broadcast(&pool->notify);
	pthread_mutex_unlock(&pool->lock);

	for (int i = 0; i < pool->num_threads; i++) {
		pthread_join(pool->threads[i], NULL);
	}

	free(pool->threads);
	pthread_mutex_destroy(&pool->lock);
	pthread_cond_destroy(&pool->notify);
	pthread_cond_destroy(&pool->working_cond);
	free(pool);
}
