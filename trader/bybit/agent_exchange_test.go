package bybit

import "testing"

func TestPositionIndexForBybitMode(t *testing.T) {
	if got := positionIndex("one-way", "buy"); got != 0 {
		t.Fatalf("one-way position index = %d, want 0", got)
	}
	if got := positionIndex("hedge", "buy"); got != 1 {
		t.Fatalf("hedge buy position index = %d, want 1", got)
	}
	if got := positionIndex("hedge", "sell"); got != 2 {
		t.Fatalf("hedge sell position index = %d, want 2", got)
	}
}
