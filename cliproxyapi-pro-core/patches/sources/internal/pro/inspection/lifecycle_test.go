package inspection

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPauseWaitsForActiveInspectionAndRejectsNewWork(t *testing.T) {
	lifecycle := &Lifecycle{}
	release, err := lifecycle.Begin()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- lifecycle.Pause(context.Background()) }()
	time.Sleep(10 * time.Millisecond)
	if _, err := lifecycle.Begin(); !errors.Is(err, ErrPaused) {
		t.Fatalf("Begin() error = %v, want ErrPaused", err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if err := lifecycle.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	release, err = lifecycle.Begin()
	if err != nil {
		t.Fatalf("Begin() after Resume error = %v", err)
	}
	release()
}

func TestCanceledPauseReopensLifecycle(t *testing.T) {
	lifecycle := &Lifecycle{}
	release, err := lifecycle.Begin()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lifecycle.Pause(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Pause() error = %v, want context canceled", err)
	}
	release()
	secondRelease, err := lifecycle.Begin()
	if err != nil {
		t.Fatalf("Begin() after canceled pause error = %v", err)
	}
	secondRelease()
}
