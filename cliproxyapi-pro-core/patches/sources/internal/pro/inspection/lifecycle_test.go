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
