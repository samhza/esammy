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

func Composite(path, outputformat string, img image.Image, under bool, point image.Point) ([]byte, error) {
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
	args = append(args, "-f", "png_pipe", "-i", "-")
	c := "[0:v][1:v]"
	if under {
		c = "[1:v][0:v]"
	}
	overlay := fmt.Sprintf("%soverlay=%d:%d", c, -point.X, -point.Y)
	filterComplex = append(filterComplex, overlay)
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
	if err := png.Encode(stdin, img); err != nil {
		return nil, errors.Wrap(err, "failed to encode png")
	}
	stdin.Close()

	err = cmd.Wait()
	if pipe {
		return outbuf.Bytes(), err
	}
	return ioutil.ReadFile(outpath)
}

func Speed(path, outputformat string, speed float64) ([]byte, error) {
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
	filterf := "setpts=%f*PTS"
	if outputformat != "gif" {
		filterf = filterf + ";[0:a]atempo=1.0/%[1]f[a]"
		args = append(args, "-map", "[a]")
	} else {
		filterComplex = append(filterComplex, "split [a][b];[a]palettegen [p];[b][p]paletteuse")
	}
	filter := fmt.Sprintf(filterf, speed)
	filterComplex = append(filterComplex, filter)
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
	stdin.Close()

	err = cmd.Wait()
	if pipe {
		return outbuf.Bytes(), err
	}
	return ioutil.ReadFile(outpath)
}
