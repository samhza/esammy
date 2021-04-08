package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Stream is the interface used to represent ffmpeg media streams.
type Stream interface {
	AddStream(*Cmd) string
}

// Video selects the video portion of a stream
func Video(s Stream) Stream { return streamPortion{s, "v"} }

// Audio selects the audio portion of a stream
func Audio(s Stream) Stream { return streamPortion{s, "a"} }

type streamPortion struct {
	s          Stream
	streamType string
}

func (s streamPortion) AddStream(c *Cmd) string {
	return s.s.AddStream(c) + ":" + s.streamType
}

type Input struct {
	Name    string
	Options []string
}

func (f Input) AddStream(c *Cmd) string {
	i := -1
Outer:
	for j, file := range c.inputFiles {
		if f.Name != file.Name {
			continue
		}
		if len(f.Options) != len(file.Options) {
			continue
		}
		for i, val := range f.Options {
			if file.Options[i] != val {
				break Outer
			}
		}
		i = j
		break
	}
	if i == -1 {
		i = len(c.inputFiles)
		c.inputFiles = append(c.inputFiles, f)
	}
	return strconv.Itoa(i)
}

type InputFile struct {
	File    *os.File
	Options []string
}

func (f InputFile) AddStream(c *Cmd) string {
	i := -1
	for j, file := range c.extraFiles {
		if f.File == file {
			i = j
			break
		}
	}
	if i == -1 {
		i = len(c.extraFiles)
		c.extraFiles = append(c.extraFiles, f.File)
	}
	return Input{"pipe:" + strconv.Itoa(i+3), f.Options}.AddStream(c)
}

type Cmd struct {
	inputFiles []Input
	extraFiles []*os.File
	filters    []filter
	outputs    []output
	ns         int
}

type filter struct {
	In     []string
	String string
	Out    []string
}

type output struct {
	Name       string
	Options    []string
	StreamSels []string
}

func (c *Cmd) AddFileOutput(name string, options []string,
	streams ...Stream) {
	o := output{name, options, make([]string, len(streams))}
	for i, s := range streams {
		if IsInputStream(s) {
			o.StreamSels[i] = s.AddStream(c)
		} else {
			o.StreamSels[i] = "[" + s.AddStream(c) + "]"
		}
	}
	c.outputs = append(c.outputs, o)
	return
}

func IsInputStream(s Stream) bool {
	switch s := s.(type) {
	case Input, InputFile:
		return true
	case streamPortion:
		return IsInputStream(s.s)
	}
	return false
}

func (c *Cmd) Cmd() *exec.Cmd {
	var args []string
	for _, in := range c.inputFiles {
		args = append(args, in.Options...)
		args = append(args, "-i", in.Name)
	}
	if len(c.filters) != 0 {
		sb := &strings.Builder{}
		for i, f := range c.filters {
			for _, i := range f.In {
				fmt.Fprintf(sb, "[%s]", i)
			}
			fmt.Fprint(sb, f.String)
			for _, i := range f.Out {
				fmt.Fprintf(sb, "[%s]", i)
			}
			if i != len(c.filters)-1 {
				sb.WriteString(";")
			}
		}
		args = append(args, "-filter_complex", sb.String())
	}
	for _, o := range c.outputs {
		for _, s := range o.StreamSels {
			args = append(args, "-map", s)
		}
		args = append(args, o.Options...)
		args = append(args, o.Name)
	}
	cmd := exec.Command("ffmpeg", args...)
	cmd.ExtraFiles = c.extraFiles
	return cmd
}

func (c *Cmd) filter(in []string, str string, n int) []string {
Outer:
	for _, f := range c.filters {
		if f.String != str {
			continue
		}
		if len(in) != len(f.In) || len(f.Out) != n {
			continue
		}
		for i, s := range f.In {
			if s != in[i] {
				continue Outer
			}
		}
		return f.Out
	}
	f := make([]string, n)
	for i := range f {
		f[i] = "s" + strconv.Itoa(c.ns)
		c.ns++
	}
	c.filters = append(c.filters, filter{in, str, f})
	return f
}

type nopFilter string

func Split(stream Stream) (Stream, Stream) {
	streams := SplitN(stream, 2)
	return streams[0], streams[1]
}

func SplitN(stream Stream, n int) []Stream {
	return split(stream, n, "split")
}

func ASplit(stream Stream) (Stream, Stream) {
	streams := ASplitN(stream, 2)
	return streams[0], streams[1]
}

func ASplitN(stream Stream, n int) []Stream {
	return split(stream, n, "asplit")
}

func split(stream Stream, n int, filter string) []Stream {
	out := make([]Stream, n)
	for i := range out {
		out[i] = splitFilter{stream, n, i, filter}
	}
	return out
}

type splitFilter struct {
	s Stream
	n int
	i int

	filter string
}

func (f splitFilter) AddStream(c *Cmd) string {
	inputs := []string{f.s.AddStream(c)}
	return c.filter(inputs, f.filter+"="+strconv.Itoa(f.n), f.n)[f.i]
}

func Concat(v, a int, streams ...Stream) []Stream {
	out := make([]Stream, len(streams))
	for i := range out {
		out[i] = concatFilter{v, a, streams, i}
	}
	return out
}

type concatFilter struct {
	v, a int
	s    []Stream
	i    int
}

func (f concatFilter) AddStream(c *Cmd) string {
	inputs := make([]string, len(f.s))
	for i, s := range f.s {
		inputs[i] = s.AddStream(c)
	}
	return c.filter(inputs, fmt.Sprintf("concat=v=%d:a=%d", f.v, f.a), f.v+f.a)[f.i]
}

type simpFilter struct {
	s      Stream
	filter string
}

func (s simpFilter) AddStream(c *Cmd) string {
	var inputs []string
	if s.s != nil {
		inputs = []string{s.s.AddStream(c)}
	}
	return c.filter(inputs, s.filter, 1)[0]
}

func Hflip(s Stream) Stream              { return simpFilter{s, "hflip"} }
func Reverse(s Stream) Stream            { return simpFilter{s, "reverse"} }
func Areverse(s Stream) Stream           { return simpFilter{s, "areverse"} }
func Filter(s Stream, str string) Stream { return simpFilter{s, str} }
func APad(s Stream) Stream               { return simpFilter{s, "apad"} }
func Volume(s Stream, vol float64) Stream {
	return simpFilter{s, fmt.Sprintf("volume=%f", vol)}
}
func MultiplyPTS(s Stream, pts float64) Stream {
	return simpFilter{s, fmt.Sprintf("setpts=%f*PTS", pts)}
}
func ATempo(s Stream, vol float64) Stream {
	return simpFilter{s, fmt.Sprintf("atempo=%f", vol)}
}

type aMix struct {
	Streams []Stream
}

func (a aMix) AddStream(c *Cmd) string {
	inputs := make([]string, len(a.Streams))
	for i, s := range a.Streams {
		inputs[i] = s.AddStream(c)
	}
	return c.filter(inputs, fmt.Sprintf("amix=inputs=%d:duration=shortest", len(inputs)), 1)[0]
}
func AMix(streams ...Stream) Stream {
	return aMix{streams}
}

type overlay struct {
	Under, Over Stream
	X, Y        int
}

func (o overlay) AddStream(c *Cmd) string {
	inputs := []string{o.Under.AddStream(c), o.Over.AddStream(c)}
	return c.filter(inputs, fmt.Sprintf("overlay=%d:%d", o.X, o.Y), 1)[0]
}

func Overlay(under, over Stream, x, y int) Stream {
	return overlay{under, over, x, y}
}

func PaletteGen(s Stream) Stream {
	return simpFilter{s, "palettegen"}
}

type paletteUse struct {
	Video, Palette Stream
}

func (p paletteUse) AddStream(c *Cmd) string {
	inputs := []string{p.Video.AddStream(c), p.Palette.AddStream(c)}
	return c.filter(inputs, "paletteuse", 1)[0]
}

func PaletteUse(v, palette Stream) Stream {
	return paletteUse{v, palette}
}

var ANullSrc Stream = Input{"anullsrc", []string{"-f", "lavfi"}}
