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

	"github.com/kkdai/youtube/v2"
	"samhza.com/esammy/memegen"
	ff "samhza.com/ffmpeg"
	"samhza.com/ytsearch"
)

type Arguments struct {
	speed        *float64
	volume       *float64
	music        string
	bt           string
	tt           string
	cap          string
	fadein       float64
	spin         int
	fadeoutstart float64
	start        float64
	end          float64
	fadeout      float64
	musicskip    float64
	musicdelay   float64
	length       int
	fadeinstart  float64
	areverse     bool
	reverse      bool
	vreverse     bool
	reverb       bool
	mute         bool
	muffle       bool
	vibrato      bool
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
		case "reverb":
			v.reverb = true
		case "muffle":
			v.muffle = true
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

func Process(arg Arguments, itype InputType, in, out *os.File) error {
	probed, err := ff.ProbeReader(in)
	if err != nil {
		return err
	}
	if _, err = in.Seek(0, 0); err != nil {
		return err
	}
	width, height := -1, -1
	var hasAudio bool
	var inDur string
	for _, stream := range probed.Streams {
		if stream.CodecType == ff.CodecTypeVideo {
			width = stream.Width
			height = stream.Height
			inDur = stream.Duration
		} else {
			hasAudio = true
		}
	}
	var v, a ff.Stream
	instream := ff.InputFile{File: in}
	if itype == InputImage {
		instream.Options = []string{"-stream_loop", "-1"}
		v = ff.Video(instream)
		v = ff.Filter(v,
			"pad=ceil(iw/2)*2:ceil(ih/2)*2,trim=duration="+strconv.Itoa(arg.length))
		a = ff.Filter(ff.ANullSrc,
			"atrim=duration="+strconv.Itoa(arg.length))
	} else {
		v = ff.Video(instream)
		if hasAudio {
			a = ff.Audio(instream)
		} else {
			a = ff.Filter(ff.ANullSrc,
				"atrim=duration="+inDur)
		}
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
		music, err := getMusicURL(arg.music)
		if err != nil {
			return err
		}
		mus := ff.Audio(ff.Input{Name: music, Options: []string{
			"-ss", fmt.Sprintf("%v", arg.musicskip),
			"-t", "600", // lets limit it to 10 minutes
		}})
		if arg.musicdelay > 0 {
			part1, part2 := ff.ASplit(a)
			part1 = ff.Filter(part1, fmt.Sprintf("atrim=end=%f,asetpts=PTS-STARTPTS", arg.musicdelay))
			part2 = ff.Filter(part2, fmt.Sprintf("atrim=start=%f,asetpts=PTS-STARTPTS", arg.musicdelay))
			part2 = ff.AMix(mus, part2)
			a = ff.Concat(2, 0, 1, part1, part2)[0]
		} else {
			a = ff.AMix(mus, a)
		}
	}
	if arg.muffle {
		a = ff.Filter(a, "lowpass=300")
	}
	if arg.reverb {
		a = ff.Filter(a, "aecho=0.8:0.9:1000:0.1")
	}
	if arg.speed != nil {
		v = ff.MultiplyPTS(v, float64(1) / *arg.speed)
		a = ff.ATempo(a, *arg.speed)
	}
	if arg.tt != "" || arg.bt != "" {
		m := image.NewRGBA(image.Rect(0, 0, width, height))
		memegen.Impact(m, arg.tt, arg.bt)
		imginput, cancel, err := imageInput(m)
		if err != nil {
			return err
		}
		defer cancel()
		v = ff.Overlay(v, imginput, 0, 0)
	}
	if arg.cap != "" {
		image, pt := memegen.Caption(width, height, arg.cap)
		imginput, cancel, err := imageInput(image)
		if err != nil {
			return err
		}
		defer cancel()
		v = ff.Overlay(imginput, v, -pt.X, -pt.Y)
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
	fcmd := &ff.Cmd{}
	outopts := []string{"-f", "mp4", "-shortest"}
	if itype == InputVideo && ff.IsInputStream(v) {
		outopts = append(outopts, "-c:v", "copy")
	}
	fcmd.AddFileOutput(out, outopts, v, a)
	cmd := fcmd.Cmd()
	cmd.Args = append(cmd.Args, "-y", "-loglevel", "error", "-shortest")
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return fmt.Errorf("exit status %d: %s",
				exitError.ExitCode(), string(stderr.String()))
		}
		return err
	}
	return nil
}

func imageInput(img image.Image) (stream ff.Stream, cancel func(), err error) {
	pR, pW, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	go func() {
		enc.Encode(pW, img)
		pW.Close()
	}()
	imginput := ff.InputFile{File: pR}
	return imginput, func() { pR.Close() }, nil
}

func probeSize(file *os.File) (width, height int, err error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-read_intervals", "%+#1", // 1 frame only
		"-select_streams", "v:0",
		"-print_format", "default=noprint_wrappers=1",
		"-show_entries", "stream=width,height", "pipe:3",
	)
	cmd.ExtraFiles = []*os.File{file}

	b, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute FFprobe: %w", err)
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

func getMusicURL(music string) (string, error) {
	ytc := new(youtube.Client)
	vid, err := ytc.GetVideo(music)
	if err != nil {
		results, err := ytsearch.Search(music)
		if err != nil {
			return "", fmt.Errorf("searching: %w", err)
		}
		if len(results) == 0 {
			return "", fmt.Errorf("no search results")
		}
		vid, err = ytc.GetVideo(results[0].ID)
		if err != nil {
			return "", fmt.Errorf("fetching video: %w", err)
		}
	}
	var format *youtube.Format
Outer:
	for _, fmt := range vid.Formats {
		switch fmt.ItagNo {
		case 249, 250, 251:
			fmt := fmt
			format = &fmt
			break Outer
		}
	}
	if format == nil {
		return "", fmt.Errorf("audio stream for video not found")
	}
	return ytc.GetStreamURL(vid, format)
}
