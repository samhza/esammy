package ff

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Size struct {
	Width  int
	Height int
}

func Probe(path string) (*Size, int, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-read_intervals", "%+#1", // 1 frame only
		"-select_streams", "v:0",
		"-print_format", "default=noprint_wrappers=1",
		"-show_entries", "stream=width,height,r_frame_rate", path,
	)

	b, err := cmd.Output()
	if err != nil {
		return nil, 0, errors.Wrap(err, "Failed to execute FFprobe")
	}

	var size Size
	var framerate int

	for _, t := range bytes.Fields(b) {
		p := bytes.Split(t, []byte("="))
		if len(p) != 2 {
			return nil, 0, fmt.Errorf("invalid line: %q", t)
		}
		v := strings.Split(string(p[1]), "/")[0]
		i, err := strconv.Atoi(v)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "Failed to parse int from line %q", t)
		}

		switch string(p[0]) {
		case "width":
			size.Width = i
		case "height":
			size.Height = i
		case "r_frame_rate":
			framerate = i
		}
	}

	return &size, framerate, nil
}
