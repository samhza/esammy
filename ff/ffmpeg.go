package ff

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

func Composite(path, outputformat string, overlay image.Image, pt image.Point, under bool) ([]byte, error) {
	var c string
	if under {
		c = "[1:v][0:v]"
	} else {
		c = "[0:v][1:v]"
	}
	pos := fmt.Sprintf("%d:%d", pt.X, -pt.Y)
	outpath := "-"
	pipe := true
	var palette string
	if outputformat == "mp4" {
		tmp, err := ioutil.TempFile("", "esammy.*.mp4")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temporary file")
		}
		defer os.Remove(tmp.Name())
		outpath = tmp.Name()
		tmp.Close()
		pipe = false
	} else if outputformat == "gif" {
		palette = ",split [a][b];[a]palettegen [p];[b][p]paletteuse"
	}
	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-v", "error",
		"-i", path,
		"-f", "png_pipe", "-i", "-",
		"-filter_complex",
		c+"overlay="+pos+palette,
		"-f", outputformat, outpath,
	)

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

	if err := png.Encode(stdin, overlay); err != nil {
		return nil, errors.Wrap(err, "failed to encode png")
	}
	stdin.Close()

	err = cmd.Wait()
	if pipe {
		return outbuf.Bytes(), err
	}
	return ioutil.ReadFile(outpath)
}
