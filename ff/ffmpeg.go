package ff

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
)

// OverlayGIF overlays an image onto a GIF
func OverlayGIF(input io.Reader, overlay image.Image) ([]byte, error) {
	var pngbuf bytes.Buffer
	if err := png.Encode(&pngbuf, overlay); err != nil {
		return nil, err
	}

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer pipeR.Close()
	defer pipeW.Close() // double close is fine

	var outbuf bytes.Buffer

	cmd := exec.Command(
		"ffmpeg",
		"-v", "error",
		"-f", "gif_pipe", "-i", "-",
		"-f", "png_pipe", "-i", "pipe:3",
		"-filter_complex", "[0:v][1:v]overlay=0:0,split [a][b];[a] palettegen [p];[b][p] paletteuse",
		"-f", "gif", "-",
	)

	cmd.Stderr = os.Stderr
	cmd.Stdout = &outbuf
	cmd.Stdin = input
	cmd.ExtraFiles = []*os.File{pipeR}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	if _, err := pngbuf.WriteTo(pipeW); err != nil {
		return nil, err
	}
	pipeW.Close()

	err = cmd.Wait()
	return outbuf.Bytes(), err
}
