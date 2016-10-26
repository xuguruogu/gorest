package gorest

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Doer executes http requests.  It is implemented by *http.Client.  You can
// wrap *http.Client with layers of Doers to form a stack of client-side
// middleware.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RestClient is an HTTP Request builder and sender.
type RestClient struct {
	// http Client for doing requests
	httpClient Doer
	// HTTP method (GET, POST, etc.)
	method string
	// raw url string for requests
	rawURL string
	// stores key-values pairs to add to request's Headers
	header http.Header
	// url tagged query structs
	data      []interface{}
	serialize func(params ...interface{}) (string, error)
}

// New returns a new RestClient with an http DefaultClient.
func New() *RestClient {
	return &RestClient{
		httpClient: http.DefaultClient,
		method:     "GET",
		header:     make(http.Header),
		data:       make([]interface{}, 0),
		serialize:  formSting,
	}
}

// New ...
func (s *RestClient) New() *RestClient {
	headerCopy := make(http.Header)
	for k, v := range s.header {
		headerCopy[k] = v
	}
	return &RestClient{
		httpClient: s.httpClient,
		method:     s.method,
		rawURL:     s.rawURL,
		header:     headerCopy,
		data:       append([]interface{}{}, s.data...),
	}
}

// JSON ...
func (s *RestClient) JSON() *RestClient {
	s.serialize = jsonSting
	return s
}

// FORM ...
func (s *RestClient) FORM() *RestClient {
	s.serialize = formSting
	return s
}

// Client ...
func (s *RestClient) Client(httpClient *http.Client) *RestClient {
	if httpClient == nil {
		return s.Doer(http.DefaultClient)
	}
	return s.Doer(httpClient)
}

// Doer sets the custom Doer implementation used to do requests.
// If a nil client is given, the http.DefaultClient will be used.
func (s *RestClient) Doer(doer Doer) *RestClient {
	if doer == nil {
		s.httpClient = http.DefaultClient
	} else {
		s.httpClient = doer
	}
	return s
}

// Method

// Head sets the RestClient method to HEAD and sets the given pathURL.
func (s *RestClient) Head(pathURL string) *RestClient {
	s.method = "HEAD"
	return s.Path(pathURL)
}

// Get sets the RestClient method to GET and sets the given pathURL.
func (s *RestClient) Get(pathURL string) *RestClient {
	s.method = "GET"
	return s.Path(pathURL)
}

// Post sets the RestClient method to POST and sets the given pathURL.
func (s *RestClient) Post(pathURL string) *RestClient {
	s.method = "POST"
	return s.Path(pathURL)
}

// Put sets the RestClient method to PUT and sets the given pathURL.
func (s *RestClient) Put(pathURL string) *RestClient {
	s.method = "PUT"
	return s.Path(pathURL)
}

// Patch sets the RestClient method to PATCH and sets the given pathURL.
func (s *RestClient) Patch(pathURL string) *RestClient {
	s.method = "PATCH"
	return s.Path(pathURL)
}

// Delete sets the RestClient method to DELETE and sets the given pathURL.
func (s *RestClient) Delete(pathURL string) *RestClient {
	s.method = "DELETE"
	return s.Path(pathURL)
}

// Header

// Add adds the key, value pair in Headers, appending values for existing keys
// to the key's values. Header keys are canonicalized.
func (s *RestClient) Add(key, value string) *RestClient {
	s.header.Add(key, value)
	return s
}

// Set sets the key, value pair in Headers, replacing existing values
// associated with key. Header keys are canonicalized.
func (s *RestClient) Set(key, value string) *RestClient {
	s.header.Set(key, value)
	return s
}

// SetBasicAuth sets the Authorization header to use HTTP Basic Authentication
// with the provided username and password. With HTTP Basic Authentication
// the provided username and password are not encrypted.
func (s *RestClient) SetBasicAuth(username, password string) *RestClient {
	return s.Set("Authorization", "Basic "+basicAuth(username, password))
}

// basicAuth returns the base64 encoded username:password for basic auth copied
// from net/http.
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// Url

// Base sets the rawURL. If you intend to extend the url with Path,
// baseUrl should be specified with a trailing slash.
func (s *RestClient) Base(rawURL string) *RestClient {
	s.rawURL = rawURL
	return s
}

// Path extends the rawURL with the given path by resolving the reference to
// an absolute URL. If parsing errors occur, the rawURL is left unmodified.
func (s *RestClient) Path(path string) *RestClient {
	baseURL, baseErr := url.Parse(s.rawURL)
	pathURL, pathErr := url.Parse(path)
	if baseErr == nil && pathErr == nil {
		s.rawURL = baseURL.ResolveReference(pathURL).String()
		return s
	}
	return s
}

// ParamStruct ...
func (s *RestClient) ParamStruct(data interface{}) *RestClient {
	if data != nil {
		s.data = append(s.data, data)
	}
	return s
}

// Param ...
func (s *RestClient) Param(key string, value interface{}) *RestClient {
	s.data = append(s.data, map[string]interface{}{key: value})
	return s
}

// Request ...
func (s *RestClient) Request() (*http.Request, error) {
	reqURL, err := url.Parse(s.rawURL)
	if err != nil {
		return nil, err
	}
	var req *http.Request
	switch s.method {
	case "GET":
		str, err := s.serialize(s.data...)
		if err != nil {
			return nil, err
		}
		reqURL.RawQuery = str
		req, err = http.NewRequest(s.method, reqURL.String(), nil)
		if err != nil {
			return nil, err
		}
	case "HEAD", "POST", "PUT", "PATCH", "DELETE":
		str, err := s.serialize(s.data...)
		if err != nil {
			return nil, err
		}
		req, err = http.NewRequest(s.method, reqURL.String(), strings.NewReader(str))
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown method: [%s]", s.method)
	}
	addHeaders(req, s.header)
	return req, nil
}

func formSting(params ...interface{}) (string, error) {
	values, err := toMap(params...)
	if err != nil {
		return "", err
	}
	return changeMapToURLValues(values).Encode(), nil
}

func jsonSting(params ...interface{}) (string, error) {
	values, err := toMap(params...)
	if err != nil {
		return "", err
	}
	d, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(d), nil
}

func toMap(params ...interface{}) (map[string]interface{}, error) {
	jsonValues := map[string]interface{}{}
	for _, data := range params {
		content, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		var val map[string]interface{}
		err = json.Unmarshal(content, &val)
		if err != nil {
			return nil, err
		}

		for k, v := range val {
			jsonValues[k] = v
		}
	}
	return jsonValues, nil

}

func changeMapToURLValues(data map[string]interface{}) url.Values {
	var newUrlValues = url.Values{}
	for k, v := range data {
		switch val := v.(type) {
		case string:
			newUrlValues.Add(k, val)
		case bool:
			newUrlValues.Add(k, strconv.FormatBool(val))
		// if a number, change to string
		// json.Number used to protect against a wrong (for GoRequest) default conversion
		// which always converts number to float64.
		// This type is caused by using Decoder.UseNumber()
		case json.Number:
			newUrlValues.Add(k, string(val))
		case int:
			newUrlValues.Add(k, strconv.FormatInt(int64(val), 10))
		// TODO add all other int-Types (int8, int16, ...)
		case float64:
			newUrlValues.Add(k, strconv.FormatFloat(float64(val), 'f', -1, 64))
		case float32:
			newUrlValues.Add(k, strconv.FormatFloat(float64(val), 'f', -1, 64))
		// following slices are mostly needed for tests
		case []string:
			for _, element := range val {
				newUrlValues.Add(k, element)
			}
		case []int:
			for _, element := range val {
				newUrlValues.Add(k, strconv.FormatInt(int64(element), 10))
			}
		case []bool:
			for _, element := range val {
				newUrlValues.Add(k, strconv.FormatBool(element))
			}
		case []float64:
			for _, element := range val {
				newUrlValues.Add(k, strconv.FormatFloat(float64(element), 'f', -1, 64))
			}
		case []float32:
			for _, element := range val {
				newUrlValues.Add(k, strconv.FormatFloat(float64(element), 'f', -1, 64))
			}
		// these slices are used in practice like sending a struct
		case []interface{}:

			if len(val) <= 0 {
				continue
			}

			switch val[0].(type) {
			case string:
				for _, element := range val {
					newUrlValues.Add(k, element.(string))
				}
			case bool:
				for _, element := range val {
					newUrlValues.Add(k, strconv.FormatBool(element.(bool)))
				}
			case json.Number:
				for _, element := range val {
					newUrlValues.Add(k, string(element.(json.Number)))
				}
			}
		default:
			// TODO add ptr, arrays, ...
		}
	}
	return newUrlValues
}

func addHeaders(req *http.Request, header http.Header) {
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}

// Receive creates a new HTTP request and returns the response. Success
// responses (2XX) are JSON decoded into the value pointed to by successV and
// other responses are JSON decoded into the value pointed to by failureV.
// Any error creating the request, sending it, or decoding the response is
// returned.
// Receive is shorthand for calling Request and Do.
func (s *RestClient) Receive(value interface{}, statusCode ...*int) error {
	req, err := s.Request()
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if len(statusCode) != 0 {
		*statusCode[0] = resp.StatusCode
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	//code
	if code := resp.StatusCode; code != 200 {
		return errors.New(string(body))
	}

	if value != nil {
		err = json.Unmarshal(body, value)
		if err != nil {
			return fmt.Errorf("parse message body err: %+v, message: %s", err, string(body))
		}
	}

	return nil
}
