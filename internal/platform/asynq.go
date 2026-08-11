package platform

import "github.com/hibiken/asynq"

// NewAsynqServer creates an asynq worker server.
func NewAsynqServer(redisURL string) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr(redisURL)},
		asynq.Config{Concurrency: 5},
	)
}

// NewAsynqScheduler creates an asynq periodic task scheduler.
func NewAsynqScheduler(redisURL string) *asynq.Scheduler {
	return asynq.NewScheduler(
		asynq.RedisClientOpt{Addr: redisAddr(redisURL)},
		nil,
	)
}

// redisAddr extracts host:port from a redis:// URL.
func redisAddr(url string) string {
	const prefix = "redis://"
	if len(url) > len(prefix) && url[:len(prefix)] == prefix {
		return url[len(prefix):]
	}
	return url
}
