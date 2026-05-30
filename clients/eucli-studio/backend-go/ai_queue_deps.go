package main

import "context"

type aiRunQueueDeps interface {
	storageSetByKey(key string, value any) error
	storageGetByKey(key string) (any, error)
	storageRemoveByKey(key string) error
	patchAssistantMessageFinal(target aiRunTarget, status string, finalText string, finishedAt int64) error
	saveBoxSessionID(roleID string, chatID string, sessionID string) error
	boxRunChat(ctx context.Context, request boxRunRequest, onDelta func(string), onSession func(string)) (boxRunResult, error)
}
