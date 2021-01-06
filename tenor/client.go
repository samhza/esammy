package tenor

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"

	"github.com/gorilla/schema"
)

type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (err HTTPError) Error() string {
	return "non-2XX status code: " + strconv.Itoa(err.StatusCode)
}

var (
	BaseEndpoint = "https://api.tenor.com"
	Version      = "1"
	Path         = "/v" + Version
	Endpoint     = BaseEndpoint + Path + "/"
)

type Client struct {
	*http.Client

	apiKey        string
	schemaEncoder *schema.Encoder
}

func NewClient(apiKey string) *Client {
	var c Client
	c.apiKey = apiKey
	c.Client = &http.Client{}
	c.schemaEncoder = schema.NewEncoder()
	return &c
}

func (c *Client) GIFs(ids []string, filter MediaFilter, limit int) ([]GIF, error) {
	if ids == nil {
		return nil, errors.New("ids == nil")
	}
	if len(ids) > 50 {
		return nil, errors.New("len(ids) > 50")
	}
	if limit > 50 {
		return nil, errors.New("limit > 20")
	}
	var gifs []GIF
	var pos string
	for {
		resp, err := c.gifs(ids, filter, limit, pos)
		if err != nil {
			return nil, err
		}
		gifs = append(gifs, resp.Results...)
		if resp.Next == "0" {
			break
		}
		pos = resp.Next
	}
	return gifs, nil
}

type gifsResponse struct {
	Results []GIF  `json:"results"`
	Next    string `json:"next"`
}

func (c *Client) gifs(ids []string, filter MediaFilter, limit int, pos string) (gifsResponse, error) {
	var params = struct {
		IDs string `schema:"ids"`

		MediaFilter MediaFilter `schema:"media_filter,omitempty"`
		Limit       int         `schema:"limit,omitempty"`
		Pos         string      `schema:"pos,omitempty"`
	}{IDs: strings.Join(ids, ",")}
	var resp gifsResponse
	err := c.requestJSON("get", Endpoint+"gifs", &resp, params)
	return resp, err
}

func (c *Client) requestJSON(method, url string, to interface{}, form interface{}) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}
	val := make(neturl.Values)
	err = c.schemaEncoder.Encode(form, val)
	if err != nil {
		return err
	}
	val.Set("key", c.apiKey)
	req.URL.RawQuery = val.Encode()
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := ioutil.ReadAll(resp.Body)
		return HTTPError{resp.StatusCode, data}
	}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(to)
	if err != nil {
		return err
	}
	return nil
}
