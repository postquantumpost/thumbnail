// thumbnail extracts candidate thumbnail frames from a video at chapter timestamps.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
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

// stamp is a chapter time: whole seconds plus a frame remainder within that second.
type stamp struct{ sec, frame int }

// parseFrame parses the "(:N)" frame remainder field.
func parseFrame(s string) (int, error) {
	if !strings.HasPrefix(s, "(:") || !strings.HasSuffix(s, ")") {
		return 0, fmt.Errorf("bad frame field %q", s)
	}
	n, err := strconv.Atoi(s[2 : len(s)-1])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("bad frame field %q", s)
	}
	return n, nil
}

func readChapters(path string) ([]stamp, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []stamp
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
		frame := 0
		if len(fields) > 1 && strings.HasPrefix(fields[1], "(") {
			frame, err = parseFrame(fields[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: skipping line %q: %v\n", path, sc.Text(), err)
				continue
			}
		}
		out = append(out, stamp{t, frame})
	}
	return out, sc.Err()
}

// lastStamp returns the seconds and frame remainder of the video's final frame.
func lastStamp(video string) (stamp, error) {
	b, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate", "-of", "default=nw=1:nk=1", video).Output()
	if err != nil {
		return stamp{}, err
	}
	num, den, ok := strings.Cut(strings.TrimSpace(string(b)), "/")
	n, err1 := strconv.ParseFloat(num, 64)
	d := 1.0
	var err2 error
	if ok {
		d, err2 = strconv.ParseFloat(den, 64)
	}
	if err1 != nil || err2 != nil || n <= 0 || d <= 0 {
		return stamp{}, fmt.Errorf("bad frame rate %q", strings.TrimSpace(string(b)))
	}
	fps := n / d

	b, err = exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time", "-of", "csv=p=0", video).Output()
	if err != nil {
		return stamp{}, err
	}
	last := -1.0
	for _, l := range strings.Fields(string(b)) {
		if v, err := strconv.ParseFloat(strings.TrimRight(l, ","), 64); err == nil && v > last {
			last = v
		}
	}
	if last < 0 {
		return stamp{}, fmt.Errorf("no video packets")
	}
	sec := int(last)
	frame := int(math.Round(last*fps)) - int(math.Ceil(float64(sec)*fps-1e-6))
	return stamp{sec, max(frame, 0)}, nil
}

func hmsName(base string, s stamp) string {
	sec := s.sec
	return fmt.Sprintf("%s_%02d_%02d_%02d_%02d.png", base, sec/3600, sec/60%60, sec%60, s.frame)
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
	last, err := lastStamp(video)
	if err != nil {
		return fmt.Errorf("ffprobe: %w", err)
	}
	lastSec := last.sec

	base := stem
	seen := map[stamp]bool{}
	var times []stamp
	add := func(t stamp) {
		if !seen[t] {
			seen[t] = true
			times = append(times, t)
		}
	}
	add(stamp{})
	for _, t := range chapters {
		add(stamp{max(t.sec+offset, 0), t.frame})
	}

	for _, t := range times {
		if t.sec > lastSec {
			fmt.Fprintf(os.Stderr, "%s: timestamp %d s beyond video end, skipping\n", video, t.sec)
			continue
		}
		out := hmsName(base, t)
		args := []string{"-ss", strconv.Itoa(t.sec), "-i", video}
		if t.frame > 0 {
			// After an accurate seek, frame n=0 is the first frame of the second.
			args = append(args, "-vf", fmt.Sprintf("select=eq(n\\,%d)", t.frame))
		}
		args = append(args, "-frames:v", "1", out)
		if err := run(args...); err != nil {
			return err
		}
		fmt.Println(out)
	}

	// Seek back from the end and keep overwriting one image so the final frame remains.
	out := hmsName(base, last)
	if !seen[last] {
		if err := run("-sseof", "-3", "-i", video, "-update", "1", "-q:v", "1", out); err != nil {
			return err
		}
		fmt.Println(out)
	}
	return nil
}

// offset in seconds added to each chapter timestamp.
var offset int

func main() {
	flag.IntVar(&offset, "offset", 0, "seconds added to each chapter timestamp")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-offset secs] file.mp4|file.txt ...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	rc := 0
	for _, a := range flag.Args() {
		if err := process(a); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", a, err)
			rc = 1
		}
	}
	os.Exit(rc)
}
