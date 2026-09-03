package inspection

import "testing"

func TestConfirmationCounterCountsAndClearsPrefix(t *testing.T) {
	counter := NewConfirmationCounter()
	if confirmed, count, _ := counter.Confirm("auth|disable|quota", 2); confirmed || count != 1 {
		t.Fatalf("first confirmation = %v, %d", confirmed, count)
	}
	counter.BeginRun()
	if confirmed, count, _ := counter.Confirm("auth|disable|quota", 2); !confirmed || count != 2 {
		t.Fatalf("second confirmation = %v, %d", confirmed, count)
	}
	counter.ClearPrefix("auth|")
	counter.BeginRun()
	if confirmed, count, _ := counter.Confirm("auth|disable|quota", 2); confirmed || count != 1 {
		t.Fatalf("cleared confirmation = %v, %d", confirmed, count)
	}
}

func TestConfirmationCounterRequiresConsecutiveRunsAndRestores(t *testing.T) {
	counter := NewConfirmationCounter()
	if confirmed, count, _ := counter.Confirm("auth|delete|invalid", 3); confirmed || count != 1 {
		t.Fatalf("first confirmation = %v, %d", confirmed, count)
	}
	if confirmed, count, _ := counter.Confirm("auth|delete|invalid", 3); confirmed || count != 1 {
		t.Fatalf("duplicate confirmation in one run = %v, %d", confirmed, count)
	}

	restored := NewConfirmationCounter()
	restored.Restore(counter.State())
	restored.BeginRun()
	if confirmed, count, _ := restored.Confirm("auth|delete|invalid", 3); confirmed || count != 2 {
		t.Fatalf("restored second run = %v, %d", confirmed, count)
	}
	restored.BeginRun()
	if confirmed, count, _ := restored.Confirm("other|delete|invalid", 3); confirmed || count != 1 {
		t.Fatalf("unrelated confirmation = %v, %d", confirmed, count)
	}
	restored.BeginRun()
	if confirmed, count, _ := restored.Confirm("auth|delete|invalid", 3); confirmed || count != 1 {
		t.Fatalf("non-consecutive confirmation = %v, %d", confirmed, count)
	}
}

func TestConfirmationCounterResetClearsPortableState(t *testing.T) {
	counter := NewConfirmationCounter()
	counter.Confirm("auth|delete|invalid", 3)
	counter.Reset()
	counter.BeginRun()
	if confirmed, count, _ := counter.Confirm("auth|delete|invalid", 3); confirmed || count != 1 {
		t.Fatalf("confirmation after reset = %v, %d", confirmed, count)
	}
}
