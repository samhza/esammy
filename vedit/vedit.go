package vedit

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"samhza.com/esammy/memegen"
	"samhza.com/esammy/vedit/ffmpeg"
	ff "samhza.com/esammy/vedit/ffmpeg"
)

type Arguments struct {
	length       int
	spin         int
	mute         bool
	reverse      bool
	areverse     bool
	vreverse     bool
	vibrato      bool
	start        float64
	end          float64
	music        string
	musicskip    float64
	musicdelay   float64
	volume       *float64
	speed        *float64
	skip         *float64
	fadein       float64
	fadeinstart  float64
	fadeout      float64
	fadeoutstart float64
	tt, bt       string
	cap          string
}

func parseTimestamp(str string) (float64, error) {
	if i := strings.IndexRune(str, ':'); i != -1 {
		splat := strings.SplitN(str, ":", 2)
		smin, ssec := splat[0], splat[1]
		min, err := strconv.Atoi(smin)
		if err != nil {
			return 0, err
		}
		sec, err := strconv.ParseFloat(ssec, 64)
		if err != nil {
			return 0, err
		}
		return float64(min*60) + sec, nil
	} else {
		return strconv.ParseFloat(str, 64)
	}
}

func (v *Arguments) Parse(args string) error {
	cmds := strings.Split(args, ",")
	for i, s := range cmds {
		cmds[i] = strings.TrimSpace(s)
	}
	var err error
	v.length = 15
	for len(cmds) > 0 {
		splat := strings.SplitN(cmds[0], " ", 2)
		var cmd, arg string
		cmd = splat[0]
		if len(splat) == 2 {
			arg = splat[1]
		}
		switch cmd {
		case "speed":
			var speed float64
			speed, err = strconv.ParseFloat(arg, 64)
			if speed < 0.5 || speed > 100 {
				err = errors.New("speed must be between 0.5 and 100")
			}
			v.speed = &speed
		case "volume":
			var volume float64
			volume, err = strconv.ParseFloat(arg, 64)
			v.volume = &volume
		case "start":
			v.start, err = parseTimestamp(arg)
		case "end":
			v.end, err = parseTimestamp(arg)
		case "mute":
			v.mute = true
		case "reverse":
			v.reverse = true
		case "areverse":
			v.areverse = true
		case "vreverse":
			v.vreverse = true
		case "vibrato":
			v.vibrato = true
		case "music":
			v.music = strings.Trim(arg, "<>")
			url, err := url.Parse(v.music)
			if err != nil {
				break
			}
			if !(url.Host == "youtu.be" || url.Host == "youtube.com") {
				break
			}
			if i, err := strconv.Atoi(url.Query().Get("t")); err == nil {
				v.musicskip = float64(i)
			}
		case "musicskip":
			v.musicskip, err = parseTimestamp(arg)
		case "musicdelay":
			v.musicdelay, err = parseTimestamp(arg)
		case "length":
			v.length, err = strconv.Atoi(arg)
		case "spin":
			v.spin, err = strconv.Atoi(arg)
		case "fadein":
			v.fadein, err = strconv.ParseFloat(arg, 64)
		case "fadeinstart":
			v.fadeinstart, err = parseTimestamp(arg)
		case "fadeout":
			v.fadein, err = strconv.ParseFloat(arg, 64)
		case "fadeoutstart":
			v.fadeoutstart, err = parseTimestamp(arg)
		case "tt":
			v.tt = arg
		case "bt":
			v.bt = arg
		case "cap", "caption":
			v.cap = arg
		default:
			err = errors.New("unknown command")
		}
		if err != nil {
			err = fmt.Errorf("parsing command \"%s\": %w", cmd, err)
			break
		}
		cmds = cmds[1:]
	}
	return err
}

type InputType int

const (
	InputVideo InputType = iota
	InputImage
)

func Process(arg Arguments, itype InputType, filename string) (string, error) {
	width, height, err := probeSize(filename)
	if err != nil {
		return "", err
	}
	var v, a ff.Stream
	instream := ff.Input{Name: filename}
	if itype == InputImage {
		instream.Options = []string{"-stream_loop", "-1"}
		v, a = ff.Video(instream), ff.Audio(instream)
		v = ff.Filter(v,
			"pad=ceil(iw/2)*2:ceil(ih/2)*2,trim=duration="+strconv.Itoa(arg.length))
		a = ff.Filter(ff.ANullSrc,
			"atrim=duration="+strconv.Itoa(arg.length))
	} else {
		v, a = ff.Video(instream), ff.Audio(instream)
	}
	if arg.mute {
		a = ff.Volume(a, 0)
	}
	var trim []string
	if arg.start > 0 {
		trim = []string{fmt.Sprintf("start=%f", arg.start)}
	}
	if arg.end > 0 {
		trim = append(trim, fmt.Sprintf("end=%f", arg.end))
	}
	if len(trim) > 0 {
		a = ff.Filter(a, "atrim="+strings.Join(trim, ":"))
		v = ff.Filter(v, "trim="+strings.Join(trim, ":"))
		a = ff.Filter(a, "asetpts=PTS-STARTPTS")
		v = ff.Filter(v, "setpts=PTS-STARTPTS")
	}
	if arg.reverse || arg.areverse {
		a = ff.Filter(a, "areverse")
	}
	if arg.reverse || arg.vreverse {
		v = ff.Filter(v, "reverse")
	}
	if arg.vibrato {
		a = ff.Filter(a, "vibrato")
	}
	if arg.music != "" {
		music, err := downloadMusic(arg.music)
		if music != "" {
			defer os.Remove(music)
		}
		if err != nil {
			return "", err
		}
		mus := ff.Audio(ff.Input{Name: music})
		if arg.musicskip > 0 {
			mus = ff.Filter(mus, fmt.Sprintf("atrim=start=%f,asetpts=PTS-STARTPTS", arg.musicskip))
		}
		if arg.musicdelay > 0 {
			part1, part2 := ff.ASplit(a)
			part1 = ff.Filter(part1, fmt.Sprintf("atrim=end=%f,asetpts=PTS-STARTPTS", arg.musicdelay))
			part2 = ff.Filter(part2, fmt.Sprintf("atrim=start=%f,asetpts=PTS-STARTPTS", arg.musicdelay))
			part2 = ff.AMix(mus, part2)
			a = ff.Concat(0, 1, part1, part2)[0]
		} else {
			a = ff.AMix(mus, a)
		}
	}
	if arg.speed != nil {
		v = ff.MultiplyPTS(v, float64(1) / *arg.speed)
		a = ff.ATempo(a, *arg.speed)
	}
	if arg.tt != "" || arg.bt != "" {
		image := memegen.Impact(width, height, arg.tt, arg.bt)
		imginput, cancel, err := imageInput(image)
		if err != nil {
			return "", err
		}
		defer cancel()
		v = ffmpeg.Overlay(v, imginput, 0, 0)
	}
	if arg.cap != "" {
		image, pt := memegen.Caption(width, height, arg.cap)
		imginput, cancel, err := imageInput(image)
		if err != nil {
			return "", err
		}
		defer cancel()
		v = ffmpeg.Overlay(imginput, v, -pt.X, -pt.Y)
	}
	if arg.volume != nil {
		a = ff.Volume(a, *arg.volume)
	}
	if arg.spin > 0 {
		v = ff.Filter(v, "rotate=t*"+strconv.Itoa(arg.spin))
	}
	if arg.fadein > 0 || arg.fadeinstart > 0 {
		fadein := arg.fadein
		if fadein == 0 {
			fadein = 5
		}
		v = ff.Filter(v, fmt.Sprintf("fade=in:duration=%f:start_time=%f", fadein, arg.fadeinstart))
	}
	if arg.fadeout > 0 || arg.fadeoutstart > 0 {
		fadeout := arg.fadeout
		if fadeout == 0 {
			fadeout = 5
		}
		v = ff.Filter(v, fmt.Sprintf("fade=out:duration=%f:start_time=%f", fadeout, arg.fadeoutstart))
	}
	f, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return "", err
	}
	f.Close()
	fcmd := &ff.Cmd{}
	outopts := []string{"-f", "mp4", "-shortest"}
	if itype == InputVideo && ff.IsInputStream(v) {
		outopts = append(outopts, "-c:v", "copy")
	}
	fcmd.AddFileOutput(f.Name(), outopts, v, a)
	cmd := fcmd.Cmd()
	cmd.Args = append(cmd.Args, "-y", "-loglevel", "error", "-shortest")
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return "", fmt.Errorf("exit status %d: %s",
				exitError.ExitCode(), string(stderr.String()))
		}
		return "", err
	}
	return f.Name(), nil
}

func imageInput(img image.Image) (stream ffmpeg.Stream, cancel func(), err error) {
	pR, pW, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	go func() {
		enc.Encode(pW, img)
		pW.Close()
	}()
	imginput := ffmpeg.InputFile{File: pR}
	return imginput, func() { pR.Close() }, nil
}

func probeSize(path string) (width, height int, err error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-read_intervals", "%+#1", // 1 frame only
		"-select_streams", "v:0",
		"-print_format", "default=noprint_wrappers=1",
		"-show_entries", "stream=width,height", path,
	)

	b, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("Failed to execute FFprobe", err)
	}

	for _, t := range bytes.Fields(b) {
		p := bytes.Split(t, []byte("="))
		if len(p) != 2 {
			return 0, 0, fmt.Errorf("invalid line: %q", t)
		}
		v := strings.Split(string(p[1]), "/")[0]
		i, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse int from line %q: %w", t, err)
		}

		switch string(p[0]) {
		case "width":
			width = i
		case "height":
			height = i
		}
	}

	return width, height, nil
}

func downloadMusic(music string) (string, error) {
	f, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return "", err
	}
	f.Close()
	cmd := exec.Command("youtube-dl",
		"--max-filesize", "10m",
		"--default-search", "ytsearch",
		"-f", "[filesize<10M]249,250,251",
		"--no-continue",
		"-o", f.Name(),
		music)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return f.Name(), fmt.Errorf("exit status %d: %s",
				exitError.ExitCode(), string(stderr.String()))
		}
		return f.Name(), err
	}
	return f.Name(), nil
}
