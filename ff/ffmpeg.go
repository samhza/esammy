package ff

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

type ProcessOptions struct {
	Image  image.Image // Image to draw over or under the input
	Under  bool        // Whether Image will be drawn under or over the input
	Point  image.Point // Where the Image/input will be placed, depending on which one is on top
	SetPTS float64
}

func Process(path, outputformat string, opts ProcessOptions) ([]byte, error) {
	args := []string{
		"-y",
		"-v", "error",
		"-i", path,
	}

	outpath := "-"
	pipe := true
	if outputformat != "gif" {
		tmp, err := ioutil.TempFile("", "esammy.*."+outputformat)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temporary file")
		}
		defer os.Remove(tmp.Name())
		outpath = tmp.Name()
		tmp.Close()
		pipe = false
	}

	var filterComplex []string
	if opts.SetPTS != 0 {
		filter := fmt.Sprintf("setpts=%f*PTS", opts.SetPTS)
		filterComplex = append(filterComplex, filter)
	}
	if opts.Image != nil {
		args = append(args, "-f", "png_pipe", "-i", "-")
		c := "[0:v][1:v]"
		if opts.Under {
			c = "[1:v][0:v]"
		}
		overlay := fmt.Sprintf("%soverlay=%d:%d", c, opts.Point.X, -opts.Point.Y)
		filterComplex = append(filterComplex, overlay)
	}
	if outputformat == "gif" {
		filterComplex = append(filterComplex, "split [a][b];[a]palettegen [p];[b][p]paletteuse")
	}
	args = append(args,
		"-filter_complex", strings.Join(filterComplex, ","),
		"-f", outputformat, outpath)
	cmd := exec.Command("ffmpeg", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create stdin pipe")
	}

	var outbuf bytes.Buffer
	cmd.Stderr = os.Stderr
	if pipe {
		cmd.Stdout = &outbuf
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start command")
	}
	if opts.Image != nil {
		if err := png.Encode(stdin, opts.Image); err != nil {
			return nil, errors.Wrap(err, "failed to encode png")
		}
	}
	stdin.Close()

	err = cmd.Wait()
	if pipe {
		return outbuf.Bytes(), err
	}
	return ioutil.ReadFile(outpath)
}
