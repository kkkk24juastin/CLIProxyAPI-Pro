package observability

import (
	"context"
	"testing"
	"time"
)

func TestPauseCreatesWriteBarrierUntilResume(t *testing.T) {
	module := New()
	release, err := module.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	paused := make(chan error, 1)
	go func() { paused <- module.Pause(context.Background()) }()
	time.Sleep(10 * time.Millisecond)
	release()
	if err := <-paused; err != nil {
		t.Fatal(err)
	}
	admitted := make(chan struct{})
	go func() {
		release, err := module.Begin(context.Background())
		if err == nil {
			release()
			close(admitted)
		}
	}()
	select {
	case <-admitted:
		t.Fatal("work passed a paused observability barrier")
	case <-time.After(20 * time.Millisecond):
	}
	if err := module.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("work did not resume")
	}
}
