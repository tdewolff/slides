package main

import (
	"context"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	changed chan string
	ctx     context.Context
}

func NewWatcher(ctx context.Context) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		watcher: watcher,
		changed: make(chan string, 5),
		ctx:     ctx,
	}, nil
}

func (w *Watcher) Close() error {
	if w.changed == nil {
		return nil
	}
	err := w.watcher.Close()
	w.changed = nil
	return err
}

func (w *Watcher) Add(filename string) error {
	return w.watcher.Add(filename)
}

func (w *Watcher) Watch() error {
	for {
		select {
		case event := <-w.watcher.Events:
			if event.Op&fsnotify.Rename != 0 || event.Op&fsnotify.Write != 0 {
				w.changed <- event.Name
			} else if event.Op&fsnotify.Remove != 0 {
				if err := w.Add(event.Name); err != nil {
					return err
				}
			}
		case err := <-w.watcher.Errors:
			return err
		case <-w.ctx.Done():
			return nil
		}
	}
}

func (w *Watcher) Changed() chan string {
	return w.changed
}
