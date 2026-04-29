package main

import "testing"

func TestParseFrameList(t *testing.T) {
	got, err := parseFrameList("0,1,1,10,30")
	if err != nil {
		t.Fatalf("parseFrameList: %v", err)
	}
	want := []int{0, 1, 10, 30}
	if len(got) != len(want) {
		t.Fatalf("len(parseFrameList) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseFrameList[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestParseFrameListRejectsDescending(t *testing.T) {
	if _, err := parseFrameList("0,10,3"); err == nil {
		t.Fatal("expected descending frame list to fail")
	}
}

func TestParseFrameListRejectsNegative(t *testing.T) {
	if _, err := parseFrameList("-1"); err == nil {
		t.Fatal("expected negative frame index to fail")
	}
}
