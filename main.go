// thumbnail extracts candidate thumbnail frames from a video at chapter timestamps.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// parseHMS parses "[[h:]m:]s" into seconds.
func parseHMS(s string) (int, error) {
	total := 0
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("bad timestamp %q", s)
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("bad timestamp %q", s)
		}
		total = total*60 + n
	}
	return total, nil
}

func readChapters(path string) ([]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		t, err := parseHMS(fields[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: skipping line %q: %v\n", path, sc.Text(), err)
			continue
		}
		out = append(out, t)
	}
	return out, sc.Err()
}

func duration(video string) (float64, error) {
	b, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", video).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
}

func hmsName(base string, sec int) string {
	return fmt.Sprintf("%s_%02d_%02d_%02d.png", base, sec/3600, sec/60%60, sec%60)
}

func run(args ...string) error {
	cmd := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func process(arg string) error {
	stem := strings.TrimSuffix(arg, filepath.Ext(arg))
	chapterFile := stem + ".txt"
	video := arg
	if strings.EqualFold(filepath.Ext(arg), ".txt") {
		video = stem + ".mp4"
	}
	if _, err := os.Stat(video); err != nil {
		return err
	}
	chapters, err := readChapters(chapterFile)
	if err != nil {
		return err
	}
	dur, err := duration(video)
	if err != nil {
		return fmt.Errorf("ffprobe: %w", err)
	}
	lastSec := int(dur)

	base := stem
	seen := map[int]bool{}
	var times []int
	add := func(t int) {
		if !seen[t] {
			seen[t] = true
			times = append(times, t)
		}
	}
	add(0)
	for _, t := range chapters {
		add(t)
	}

	for _, t := range times {
		if t > lastSec {
			fmt.Fprintf(os.Stderr, "%s: timestamp %d s beyond video end, skipping\n", video, t)
			continue
		}
		out := hmsName(base, t)
		if err := run("-ss", strconv.Itoa(t), "-i", video, "-frames:v", "1", out); err != nil {
			return err
		}
		fmt.Println(out)
	}

	// Seek back from the end and keep overwriting one image so the final frame remains.
	out := hmsName(base, lastSec)
	if !seen[lastSec] {
		if err := run("-sseof", "-3", "-i", video, "-update", "1", "-q:v", "1", out); err != nil {
			return err
		}
		fmt.Println(out)
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s file.mp4|file.txt ...\n", os.Args[0])
		os.Exit(2)
	}
	rc := 0
	for _, a := range os.Args[1:] {
		if err := process(a); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", a, err)
			rc = 1
		}
	}
	os.Exit(rc)
}
