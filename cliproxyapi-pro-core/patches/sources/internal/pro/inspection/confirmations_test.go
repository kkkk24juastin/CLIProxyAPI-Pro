package inspection

import "testing"

func TestConfirmationCounterCountsAndClearsPrefix(t *testing.T) {
	counter := NewConfirmationCounter()
	if confirmed, count, _ := counter.Confirm("auth|disable|quota", 2); confirmed || count != 1 {
		t.Fatalf("first confirmation = %v, %d", confirmed, count)
	}
	if confirmed, count, _ := counter.Confirm("auth|disable|quota", 2); !confirmed || count != 2 {
		t.Fatalf("second confirmation = %v, %d", confirmed, count)
	}
	counter.ClearPrefix("auth|")
	if confirmed, count, _ := counter.Confirm("auth|disable|quota", 2); confirmed || count != 1 {
		t.Fatalf("cleared confirmation = %v, %d", confirmed, count)
	}
}
