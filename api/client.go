package api

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

type Client struct {
	client        *resty.Client
	uploadClient  *resty.Client
	contentClient *resty.Client
	baseURL       string
	token         string
	version       string
	macAddr       string
}

const (
	defaultRequestTimeout = 60 * time.Second
	noteUploadTimeout     = 10 * time.Minute
	noteContentTimeout    = 5 * time.Minute
)

func NewClient() *Client {
	return &Client{
		client:        resty.New().SetTimeout(defaultRequestTimeout),
		uploadClient:  resty.New().SetTimeout(noteUploadTimeout),
		contentClient: resty.New().SetTimeout(noteContentTimeout),
		version:       "linux_amd64_2.0",
	}
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) SetHost(host string) {
	c.baseURL = host
	c.client.SetBaseURL(host + "/api2")
	c.uploadClient.SetBaseURL(host + "/api2")
	c.contentClient.SetBaseURL(host + "/api2")
}

func (c *Client) SetMacAddr(addr string) {
	c.macAddr = addr
}

func (c *Client) GetBaseURL() string {
	return c.baseURL
}

func (c *Client) url(path string) string {
	return fmt.Sprintf("%s/%s", c.baseURL, path)
}

func (c *Client) commonParams() map[string]string {
	params := map[string]string{
		"token": c.token,
	}
	if c.macAddr != "" {
		params["macAddr"] = c.macAddr
	}
	if c.version != "" {
		params["v"] = c.version
	}
	return params
}

func (c *Client) redactError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if c.token != "" {
		message = strings.ReplaceAll(message, c.token, "[redacted]")
	}
	return fmt.Errorf("%s", message)
}

func (c *Client) get(path string, params map[string]string) (*resty.Response, error) {
	return c.getWithClient(c.client, path, params)
}

func (c *Client) getWithClient(client *resty.Client, path string, params map[string]string) (*resty.Response, error) {
	allParams := c.commonParams()
	for k, v := range params {
		allParams[k] = v
	}

	resp, err := client.R().SetQueryParams(allParams).Get(path)
	if err != nil {
		err = c.redactError(err)
		logrus.Errorf("API GET %s error: %v", path, err)
		return nil, err
	}

	logrus.Debugf("API GET %s status: %d", path, resp.StatusCode())
	if resp.IsError() {
		logrus.Errorf("API GET %s failed: status=%d body=%s", path, resp.StatusCode(), strings.TrimSpace(resp.String()))
	}
	return resp, nil
}

func (c *Client) post(path string, data interface{}, params map[string]string) (*resty.Response, error) {
	return c.postWithClient(c.client, path, data, params)
}

// postNoteUpload uses a separate, longer timeout for note writes. New notes
// can contain the full body plus base64-encoded local images and attachments;
// the server also persists and converts those files before sending headers.
// Do not retry these non-idempotent requests automatically: the server may
// have committed the note even when the response was lost.
func (c *Client) postNoteUpload(path string, data interface{}, params map[string]string) (*resty.Response, error) {
	return c.postWithClient(c.uploadClient, path, data, params)
}

func (c *Client) postWithClient(client *resty.Client, path string, data interface{}, params map[string]string) (*resty.Response, error) {
	allParams := c.commonParams()
	for k, v := range params {
		allParams[k] = v
	}

	req := client.R().SetQueryParams(allParams)
	if data != nil {
		form := flattenFormData(data)
		if path == "note/addNote" || path == "note/updateNote" {
			var payloadBytes int
			for key, value := range form {
				payloadBytes += len(key) + len(value)
			}
			logrus.Debugf("API POST %s form payload: approximately %d bytes", path, payloadBytes)
		}
		req.SetFormData(form)
	}
	resp, err := req.Post(path)
	if err != nil {
		err = c.redactError(err)
		logrus.Errorf("API POST %s error: %v", path, err)
		return nil, err
	}

	logrus.Debugf("API POST %s status: %d", path, resp.StatusCode())
	if resp.IsError() {
		logrus.Errorf("API POST %s failed: status=%d body=%s", path, resp.StatusCode(), strings.TrimSpace(resp.String()))
	}
	return resp, nil
}

// flattenFormData encodes the form shape used by the original Leanote API.
// In particular, Revel binds fields such as Tags[0] and
// Files[0][LocalFileId]; sending the same map as JSON leaves those fields
// empty and makes updateNote report noteIdNotExists.
func flattenFormData(data interface{}) map[string]string {
	out := make(map[string]string)
	var walk func(string, reflect.Value)
	walk = func(prefix string, value reflect.Value) {
		if !value.IsValid() {
			return
		}
		for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return
			}
			value = value.Elem()
		}
		switch value.Kind() {
		case reflect.Map:
			iter := value.MapRange()
			for iter.Next() {
				key := fmt.Sprint(iter.Key().Interface())
				name := key
				if prefix != "" {
					name = prefix + "[" + key + "]"
				}
				walk(name, iter.Value())
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < value.Len(); i++ {
				name := fmt.Sprintf("%s[%d]", prefix, i)
				walk(name, value.Index(i))
			}
		case reflect.Struct:
			typeOfValue := value.Type()
			for i := 0; i < value.NumField(); i++ {
				field := typeOfValue.Field(i)
				if field.PkgPath != "" { // unexported
					continue
				}
				name := field.Name
				if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
					name = tag
				}
				if prefix != "" {
					name = prefix + "[" + name + "]"
				}
				walk(name, value.Field(i))
			}
		default:
			if prefix != "" {
				out[prefix] = fmt.Sprint(value.Interface())
			}
		}
	}
	walk("", reflect.ValueOf(data))
	return out
}

func (c *Client) postFiles(path string, formData map[string]string, files map[string]string) (*resty.Response, error) {
	allParams := c.commonParams()
	for k, v := range formData {
		allParams[k] = v
	}

	req := c.client.R().SetQueryParams(allParams)

	for fieldName, filePath := range files {
		req = req.SetFile(fieldName, filePath)
	}

	resp, err := req.Post(path)
	if err != nil {
		err = c.redactError(err)
		logrus.Errorf("API POST files %s error: %v", path, err)
		return nil, err
	}

	logrus.Debugf("API POST files %s status: %d", path, resp.StatusCode())
	return resp, nil
}
