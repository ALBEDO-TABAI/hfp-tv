package platform

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerSurvivesPanickingTaskAndLoop(t *testing.T) {
	var worker Worker
	worker.Start()
	done := make(chan struct{})
	if !worker.GoTask(func() { panic("task boom") }) {
		t.Fatal("task not scheduled")
	}
	var loops atomic.Int32
	worker.GoLoop(func(ctx context.Context) {
		if loops.Add(1) == 1 {
			panic("loop boom")
		}
		close(done)
		<-ctx.Done()
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop was not restarted after a panic")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := worker.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
