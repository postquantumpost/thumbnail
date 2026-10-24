// thumbnail extracts candidate thumbnail frames from a video at chapter timestamps.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	vid "pygorvid/vid"
)

// parseHMS parses "[[h:]m:]s" into seconds.
func parseHMS(s string) (int, error) {
	total := 0
	parts := strings.Split(strings.ReplaceAll(s, "։", ":"), ":")
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

const superscriptDigits = "⁰¹²³⁴⁵⁶⁷⁸⁹"

var frameDigits = strings.NewReplacer(
	"⁰", "0", "¹", "1", "²", "2", "³", "3", "⁴", "4",
	"⁵", "5", "⁶", "6", "⁷", "7", "⁸", "8", "⁹", "9",
)

// parseFrame parses a superscript or legacy "(:N)" frame remainder.
func parseFrame(s string) (int, error) {
	digits := frameDigits.Replace(s)
	if strings.HasPrefix(s, "(:") && strings.HasSuffix(s, ")") {
		digits = s[2 : len(s)-1]
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("bad frame field %q", s)
	}
	return n, nil
}

func parseChapter(line string) (stamp, error) {
	fields := strings.Fields(line)
	if start := strings.IndexAny(line, superscriptDigits); start >= 0 {
		fields = strings.Fields(line[:start])
		if len(fields) == 0 {
			return stamp{}, fmt.Errorf("missing timestamp before frame number")
		}
		digits := line[start:]
		if end := strings.IndexFunc(digits, func(r rune) bool {
			return !strings.ContainsRune(superscriptDigits, r)
		}); end >= 0 {
			digits = digits[:end]
		}
		sec, err := parseHMS(fields[0])
		if err != nil {
			sec, err = parseHMS(fields[len(fields)-1])
			if err != nil {
				return stamp{}, err
			}
		}
		frame, err := parseFrame(digits)
		return stamp{sec, frame}, err
	}
	if len(fields) == 0 {
		return stamp{}, fmt.Errorf("missing timestamp")
	}
	sec, err := parseHMS(fields[0])
	if err != nil {
		return stamp{}, err
	}
	frame := 0
	if len(fields) > 1 && strings.HasPrefix(fields[1], "(") {
		frame, err = parseFrame(fields[1])
	}
	return stamp{sec, frame}, err
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
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		t, err := parseChapter(sc.Text())
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: skipping line %q: %v\n", path, sc.Text(), err)
			continue
		}
		out = append(out, t)
	}
	return out, sc.Err()
}

type videoFrame struct {
	index uint64
	pts   int64
}

func readVideoFrames(reader *vid.VideoSampleReader) ([]videoFrame, error) {
	frames := make([]videoFrame, 0, reader.SampleCount())
	for index := 0; index < reader.SampleCount(); index++ {
		sample, err := reader.SampleAt(uint64(index))
		if err != nil {
			return nil, err
		}
		frames = append(frames, videoFrame{pts: sample.PTSTicks})
	}
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].pts < frames[j].pts })
	for index := range frames {
		frames[index].index = uint64(index)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no video frames")
	}
	return frames, nil
}

// lastStamp returns the seconds and frame remainder of the video's final frame.
func lastStamp(frames []videoFrame, timescale uint32, fps float64) stamp {
	last := float64(frames[len(frames)-1].pts) / float64(timescale)
	sec := int(last)
	frame := int(math.Round(last*fps)) - int(math.Ceil(float64(sec)*fps-1e-6))
	return stamp{sec, max(frame, 0)}
}

func frameAtOrAfter(frames []videoFrame, seconds float64, timescale uint32) (uint64, bool) {
	ticks := seconds * float64(timescale)
	index := sort.Search(len(frames), func(i int) bool {
		return float64(frames[i].pts) >= ticks-1e-6
	})
	if index == len(frames) {
		return 0, false
	}
	return frames[index].index, true
}

func hmsName(base string, s stamp) string {
	sec := s.sec
	return fmt.Sprintf("%s_%02d_%02d_%02d_%02d.png", base, sec/3600, sec/60%60, sec%60, s.frame)
}

func saveFrame(decoder *vid.H264Decoder, index uint64, output string) error {
	frame, err := decoder.DecodeFrame(index)
	if err != nil {
		return fmt.Errorf("decode frame %d: %w", index, err)
	}
	pixels, err := frame.ConvertToRGB(vid.ColorRangeLimited)
	if err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	if err := vid.NewPNGEncoder().Write(file, pixels); err != nil {
		file.Close()
		return err
	}
	return file.Close()
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
	metadata := vid.NewMp4File()
	if !metadata.Open(video) {
		return fmt.Errorf("pygorvid: %s", metadata.ErrorInfo())
	}
	info := metadata.GetBasicInfo()
	metadataError := metadata.ErrorInfo()
	metadata.Close()
	if metadataError != "" {
		return fmt.Errorf("pygorvid: %s", metadataError)
	}
	if !info.HasVideo || len(info.VideoStreams) == 0 {
		return fmt.Errorf("pygorvid: no video stream")
	}
	stream := info.VideoStreams[0]
	if stream.FrameRate <= 0 {
		return fmt.Errorf("pygorvid: invalid video frame rate %g", stream.FrameRate)
	}
	reader, err := vid.OpenVideoSampleReader(video)
	if err != nil {
		return fmt.Errorf("pygorvid: %w", err)
	}
	defer reader.Close()
	frames, err := readVideoFrames(reader)
	if err != nil {
		return fmt.Errorf("pygorvid: %w", err)
	}
	timescale := reader.Configuration().Timescale
	if timescale == 0 {
		return fmt.Errorf("pygorvid: invalid video timescale")
	}
	last := lastStamp(frames, timescale, stream.FrameRate)
	lastSec := last.sec
	decoder := vid.NewH264Decoder(reader)

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
		index, ok := frameAtOrAfter(frames, float64(t.sec)+float64(t.frame)/stream.FrameRate, timescale)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s: timestamp %d s beyond video end, skipping\n", video, t.sec)
			continue
		}
		if err := saveFrame(decoder, index, out); err != nil {
			return err
		}
		fmt.Println(out)
	}

	// Keep the final frame as a candidate thumbnail.
	out := hmsName(base, last)
	if !seen[last] {
		if err := saveFrame(decoder, frames[len(frames)-1].index, out); err != nil {
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
