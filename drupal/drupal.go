package drupal

import (
	"derivative-ms/drupal/request"
	"fmt"
	"github.com/cristalhq/jwt/v4"
	"io"
	"io/ioutil"
	"net/http"
)

var DefaultClient = &http.Client{}

type HttpImpl struct {
	HttpClient *http.Client
}

func (h HttpImpl) Put(reqCtx request.Context, uri string, body io.ReadCloser) (int, error) {
	return put(h.HttpClient, uri, body, reqCtx)
}

func (h HttpImpl) Get(reqCtx request.Context, uri string) (io.ReadCloser, error) {
	return get(h.HttpClient, uri, reqCtx)
}

func put(h *http.Client, uri string, body io.ReadCloser, reqCtx request.Context) (int, error) {
	var (
		statusCode   int
		statusMsg    string
		responseBody io.ReadCloser
		err          error
	)

	if responseBody, statusCode, statusMsg, err = doRequest(h, http.MethodPut, uri, reqCtx.Token(), body, reqCtx.Headers()); err != nil {
		return statusCode, err
	} else {
		defer func() {
			io.Copy(ioutil.Discard, responseBody)
			responseBody.Close()
		}()
	}

	if statusCode < 200 || statusCode >= 300 {
		return statusCode, fmt.Errorf("drupal: error performing PUT %s: status code '%d', message: '%s'",
			uri, statusCode, statusMsg)
	}

	return statusCode, nil
}

func get(h *http.Client, uri string, ctx request.Context) (io.ReadCloser, error) {
	var (
		statusCode   int
		statusMsg    string
		responseBody io.ReadCloser
		err          error
	)

	if responseBody, statusCode, statusMsg, err = doRequest(h, http.MethodGet, uri, ctx.Token(), nil, ctx.Headers()); err != nil {
		return nil, err
	}

	if statusCode < 200 || statusCode >= 300 {
		defer func() {
			io.Copy(ioutil.Discard, responseBody)
			responseBody.Close()
		}()
		return responseBody, fmt.Errorf("drupal: error performing GET %s: status code '%d', message: '%s'",
			uri, statusCode, statusMsg)
	}

	return responseBody, nil
}

func doRequest(h *http.Client, method, uri string, authToken *jwt.Token, body io.ReadCloser, headers map[string]string) (responseBody io.ReadCloser, statusCode int, statusMessage string, err error) {
	var (
		req *http.Request
		res *http.Response
	)

	req, err = http.NewRequest(method, uri, body)
	if err != nil {
		return nil, -1, "", err
	} else {
		req.Close = true
		if authToken != nil {
			req.Header.Set("Authorization", asBearer(authToken))
		}
		for header, value := range headers {
			req.Header.Set(header, value)
		}
	}

	if res, err = h.Do(req); err != nil {
		return nil, -1, "", err
	}

	return res.Body, res.StatusCode, res.Status, nil
}

func asBearer(token *jwt.Token) string {
	return fmt.Sprintf("Bearer %s", token)
}
