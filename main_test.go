package main

import (
	"reflect"
	"testing"
)

func TestParseChapter(t *testing.T) {
	tests := []struct {
		name string
		line string
		want stamp
	}{
		{"place annotation", "  Three Sisters 0:06 ³⁶ ᵖˡᵃᶜᵉ²", stamp{6, 36}},
		{"region annotation", "  Three Sisters 0:08 ²⁸ ʳᵉᵍⁱᵒⁿ", stamp{8, 28}},
		{"all digits", "Chapter 1:02:03 ¹²³⁴⁵⁶⁷⁸⁹⁰", stamp{3723, 1234567890}},
		{"zero frame", "Chapter 2:03 ⁰", stamp{123, 0}},
		{"adjacent frame", "Chapter 0:06³⁶", stamp{6, 36}},
		{"multiple groups", "Chapter 0:06 ³⁶ ⁹⁹", stamp{6, 36}},
		{"legacy frame", "0:06 (:36) Three Sisters", stamp{6, 36}},
		{"legacy timestamp", "0:06 Three Sisters", stamp{6, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChapter(tt.line)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("parseChapter(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseChapterInvalid(t *testing.T) {
	for _, line := range []string{
		"", "³⁶", "Chapter invalid ³⁶", "Chapter -1:06 ³⁶",
		"Chapter 0:06", "0:06 (:bad)", "0:06 (:36",
	} {
		t.Run(line, func(t *testing.T) {
			if got, err := parseChapter(line); err == nil {
				t.Fatalf("parseChapter(%q) = %v, want an error", line, got)
			}
		})
	}
}

func TestReadChaptersSample(t *testing.T) {
	got, err := readChapters("samples/place1.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := []stamp{{6, 36}, {8, 28}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readChapters sample = %v, want %v", got, want)
	}
}
