package tools

import (
	"context"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

func WatchForConfigChangeEvents(ctx context.Context, configFilePath string, logger logr.Logger, eventChan chan string) error {
	logger.V(1).Info("Starting config watcher")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	defer watcher.Close()

	err = watcher.Add(configFilePath)
	if err != nil {
		return err
	}

	eventChan <- configFilePath

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			logger.V(1).Info("config change event", "event", event)

			if isWrite(event) || isCreate(event) {
				eventChan <- configFilePath
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}

			logger.Error(err, "config watcher error")

		case <-ctx.Done():
			logger.V(1).Info("Stopping config watcher")
			return nil
		}
	}
}

func isWrite(event fsnotify.Event) bool {
	return event.Op&fsnotify.Write == fsnotify.Write
}

func isCreate(event fsnotify.Event) bool {
	return event.Op&fsnotify.Create == fsnotify.Create
}
