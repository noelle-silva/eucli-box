package main

import "context"

type aiRunQueueDeps interface {
	runtimeSet(key string, value any)
	runtimeGet(key string) any
	runtimeRemove(key string)
	boxRunChat(ctx context.Context, request boxRunRequest, onDelta func(string), onSession func(string)) (boxRunResult, error)
}
