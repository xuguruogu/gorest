package gorest

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	goquery "github.com/google/go-querystring/query"
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
	queryStructs []interface{}
	// body provider
	bodyProvider BodyProvider
}

// New returns a new RestClient with an http DefaultClient.
func New() *RestClient {
	return &RestClient{
		httpClient:   http.DefaultClient,
		method:       "GET",
		header:       make(http.Header),
		queryStructs: make([]interface{}, 0),
	}
}

// New returns a copy of a RestClient for creating a new RestClient with properties
// from a parent RestClient. For example,
//
// 	parentSling := sling.New().Client(client).Base("https://api.io/")
// 	fooSling := parentSling.New().Get("foo/")
// 	barSling := parentSling.New().Get("bar/")
//
// fooSling and barSling will both use the same client, but send requests to
// https://api.io/foo/ and https://api.io/bar/ respectively.
//
// Note that query and body values are copied so if pointer values are used,
// mutating the original value will mutate the value within the child RestClient.
func (s *RestClient) New() *RestClient {
	// copy Headers pairs into new Header map
	headerCopy := make(http.Header)
	for k, v := range s.header {
		headerCopy[k] = v
	}
	return &RestClient{
		httpClient:   s.httpClient,
		method:       s.method,
		rawURL:       s.rawURL,
		header:       headerCopy,
		queryStructs: append([]interface{}{}, s.queryStructs...),
		bodyProvider: s.bodyProvider,
	}
}

// Http Client

// Client sets the http Client used to do requests. If a nil client is given,
// the http.DefaultClient will be used.
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

// QueryStruct appends the queryStruct to the RestClient's queryStructs. The value
// pointed to by each queryStruct will be encoded as url query parameters on
// new requests (see Request()).
// The queryStruct argument should be a pointer to a url tagged struct. See
// https://godoc.org/github.com/google/go-querystring/query for details.
func (s *RestClient) QueryStruct(queryStruct interface{}) *RestClient {
	if queryStruct != nil {
		s.queryStructs = append(s.queryStructs, queryStruct)
	}
	return s
}

// Body

// Body sets the RestClient's body. The body value will be set as the Body on new
// requests (see Request()).
// If the provided body is also an io.Closer, the request Body will be closed
// by http.Client methods.
func (s *RestClient) Body(body io.Reader) *RestClient {
	if body == nil {
		return s
	}
	return s.BodyProvider(bodyProvider{body: body})
}

// BodyProvider sets the RestClient's body provider.
func (s *RestClient) BodyProvider(body BodyProvider) *RestClient {
	if body == nil {
		return s
	}
	s.bodyProvider = body

	ct := body.ContentType()
	if ct != "" {
		s.Set(contentType, ct)
	}

	return s
}

// BodyJSON sets the RestClient's bodyJSON. The value pointed to by the bodyJSON
// will be JSON encoded as the Body on new requests (see Request()).
// The bodyJSON argument should be a pointer to a JSON tagged struct. See
// https://golang.org/pkg/encoding/json/#MarshalIndent for details.
func (s *RestClient) BodyJSON(bodyJSON interface{}) *RestClient {
	if bodyJSON == nil {
		return s
	}
	return s.BodyProvider(jsonBodyProvider{payload: bodyJSON})
}

// BodyForm sets the RestClient's bodyForm. The value pointed to by the bodyForm
// will be url encoded as the Body on new requests (see Request()).
// The bodyForm argument should be a pointer to a url tagged struct. See
// https://godoc.org/github.com/google/go-querystring/query for details.
func (s *RestClient) BodyForm(bodyForm interface{}) *RestClient {
	if bodyForm == nil {
		return s
	}
	return s.BodyProvider(formBodyProvider{payload: bodyForm})
}

// Requests

// Request returns a new http.Request created with the RestClient properties.
// Returns any errors parsing the rawURL, encoding query structs, encoding
// the body, or creating the http.Request.
func (s *RestClient) Request() (*http.Request, error) {
	reqURL, err := url.Parse(s.rawURL)
	if err != nil {
		return nil, err
	}

	err = addQueryStructs(reqURL, s.queryStructs)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if s.bodyProvider != nil {
		body, err = s.bodyProvider.Body()
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(s.method, reqURL.String(), body)
	if err != nil {
		return nil, err
	}
	addHeaders(req, s.header)
	return req, err
}

// addQueryStructs parses url tagged query structs using go-querystring to
// encode them to url.Values and format them onto the url.RawQuery. Any
// query parsing or encoding errors are returned.
func addQueryStructs(reqURL *url.URL, queryStructs []interface{}) error {
	urlValues, err := url.ParseQuery(reqURL.RawQuery)
	if err != nil {
		return err
	}
	// encodes query structs into a url.Values map and merges maps
	for _, queryStruct := range queryStructs {
		queryValues, err := goquery.Values(queryStruct)
		if err != nil {
			return err
		}
		for key, values := range queryValues {
			for _, value := range values {
				urlValues.Add(key, value)
			}
		}
	}
	// url.Values format to a sorted "url encoded" string, e.g. "key=val&foo=bar"
	reqURL.RawQuery = urlValues.Encode()
	return nil
}

// addHeaders adds the key, value pairs from the given http.Header to the
// request. Values for existing keys are appended to the keys values.
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
func (s *RestClient) Receive(value interface{}, result ...interface{}) error {
	req, err := s.Request()
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	//code
	if code := resp.StatusCode; code < 200 || code > 299 {
		if len(result) != 0 {
			json.Unmarshal(body, result)
		}
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
