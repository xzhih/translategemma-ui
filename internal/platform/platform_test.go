package platform

import (
	"reflect"
	"testing"
)

func TestRuntimeFileCandidatesWindows(t *testing.T) {
	got := RuntimeFileCandidates("windows", "translategemma-4b-it.Q4_K_M.llamafile")
	want := []string{
		"translategemma-4b-it.Q4_K_M.llamafile.exe",
		"translategemma-4b-it.Q4_K_M.llamafile",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestRuntimeFileCandidatesNonWindows(t *testing.T) {
	got := RuntimeFileCandidates("linux", "translategemma-4b-it.Q4_K_M.llamafile")
	want := []string{"translategemma-4b-it.Q4_K_M.llamafile"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
