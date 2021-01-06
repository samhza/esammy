package tenor

import (
	"encoding/json"
	"errors"
)

type GIFFormat string

const (
	FormatMP4       GIFFormat = "mp4"
	FormatLoopedMP4 GIFFormat = "loopedmp4"
	FormatTinyMP4   GIFFormat = "tinymp4"
	FormatNanoMP4   GIFFormat = "nanomp4"

	FormatGIF       GIFFormat = "gif"
	FormatMediumGIF GIFFormat = "mediumgif"
	FormatTinyGIF   GIFFormat = "tinygif"
	FormatNanoGIF   GIFFormat = "nanogif"

	FormatWebM     GIFFormat = "webm"
	FormatTinyWebM GIFFormat = "tinywebm"
	FormatNanoWebM GIFFormat = "nanowebm"
)

type MediaFilter string

const (
	MediaFilterBasic   = "basic"
	MediaFilterMinimal = "minimal"
	MediaFilterAll     = ""
)

type GIF struct {
	Created    float64               `json:"created"`
	HasAudio   bool                  `json:"hasaudio"`
	ID         string                `json:"id"`
	Media      []map[GIFFormat]Media `json:"media"`
	Tags       []string              `json:"tags"`
	Title      string                `json:"title"`
	ItemURL    string                `json:"itemurl"`
	HasCaption bool                  `json:"hascaption"`
	URL        string                `json:"url"`
}

type Media struct {
	Preview    string     `json:"preview"`
	URL        string     `json:"url"`
	Dimensions Dimensions `json:"dims"`
	Size       int        `json:"size"`
}

type Dimensions struct {
	Width, Height int
}

func (d *Dimensions) UnmarshalJSON(data []byte) error {
	var dims []int
	err := json.Unmarshal(data, &dims)
	if err != nil {
		return err
	}
	if len(dims) != 2 {
		return errors.New("len(dims) != 2")
	}
	d.Width, d.Height = dims[0], dims[1]
	return nil
}
