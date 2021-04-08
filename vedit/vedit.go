package vedit

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"git.sr.ht/~samhza/esammy/vedit/ffmpeg"
)

type Arguments struct {
	length     int
	mute       bool
	reverse    bool
	vibrato    bool
	start      float64
	end        float64
	music      string
	musicskip  float64
	musicdelay float64
	volume     *float64
	speed      *float64
	skip       *float64
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
		}
		if err != nil {
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
	var v, a ffmpeg.Stream
	instream := ffmpeg.Input{Name: filename}
	if itype == InputImage {
		instream.Options = []string{"-stream_loop", "-1"}
		v, a = ffmpeg.Video(instream), ffmpeg.Audio(instream)
		v = ffmpeg.Filter(v,
			"pad=ceil(iw/2)*2:ceil(ih/2)*2,trim=duration="+strconv.Itoa(arg.length))
		a = ffmpeg.Filter(ffmpeg.ANullSrc,
			"atrim=duration="+strconv.Itoa(arg.length))
	} else {
		v, a = ffmpeg.Video(instream), ffmpeg.Audio(instream)
	}
	if arg.mute {
		a = ffmpeg.Volume(a, 0)
	}
	var trim []string
	if arg.start > 0 {
		trim = []string{fmt.Sprintf("start=%f", arg.start)}
	}
	if arg.end > 0 {
		trim = append(trim, fmt.Sprintf("end=%f", arg.end))
	}
	if len(trim) > 0 {
		a = ffmpeg.Filter(a, "atrim="+strings.Join(trim, ":"))
		v = ffmpeg.Filter(v, "trim="+strings.Join(trim, ":"))
		a = ffmpeg.Filter(a, "asetpts=PTS-STARTPTS")
		v = ffmpeg.Filter(v, "setpts=PTS-STARTPTS")
	}
	if arg.reverse {
		a = ffmpeg.Filter(a, "areverse")
		v = ffmpeg.Filter(v, "reverse")
	}
	if arg.vibrato {
		a = ffmpeg.Filter(a, "vibrato")
	}
	if arg.music != "" {
		music, err := downloadMusic(arg.music)
		if music != "" {
			defer os.Remove(music)
		}
		if err != nil {
			return "", err
		}
		mus := ffmpeg.Audio(ffmpeg.Input{Name: music})
		if arg.musicskip > 0 {
			mus = ffmpeg.Filter(mus, fmt.Sprintf("atrim=start=%f,asetpts=PTS-STARTPTS", arg.musicskip))
		}
		if arg.musicdelay > 0 {
			part1, part2 := ffmpeg.ASplit(a)
			part1 = ffmpeg.Filter(part1, fmt.Sprintf("atrim=end=%f,asetpts=PTS-STARTPTS", arg.musicdelay))
			part2 = ffmpeg.Filter(part2, fmt.Sprintf("atrim=start=%f,asetpts=PTS-STARTPTS", arg.musicdelay))
			part2 = ffmpeg.AMix(mus, part2)
			a = ffmpeg.Concat(0, 1, part1, part2)[0]
		} else {
			a = ffmpeg.AMix(mus, a)
		}
	}
	if arg.speed != nil {
		v = ffmpeg.MultiplyPTS(v, float64(1) / *arg.speed)
		a = ffmpeg.ATempo(a, *arg.speed)
	}
	if arg.volume != nil {
		a = ffmpeg.Volume(a, *arg.volume)
	}

	f, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return "", err
	}
	f.Close()
	fcmd := &ffmpeg.Cmd{}
	outopts := []string{"-f", "mp4", "-shortest"}
	if itype == InputVideo && ffmpeg.IsInputStream(v) {
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

func downloadMusic(music string) (string, error) {
	f, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return "", err
	}
	f.Close()
	cmd := exec.Command("youtube-dl",
		"--default-search", "ytsearch",
		"--no-continue", "-f", "249,250,251", "-o", f.Name(), music)
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
