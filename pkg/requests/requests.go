package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/pinpt/go-common/event"
)

type RetryableRequest struct {
	MaxAttempts float64
	MaxDuration time.Duration
	RetryDelay  time.Duration
}
type Requests struct {
	Logger    hclog.Logger
	Client    *http.Client
	Retryable RetryableRequest
}

func New(logger hclog.Logger, client *http.Client) Requests {
	req := Requests{
		Logger: logger,
		Client: client,
	}
	return req
}

func NewRetryableDefault(logger hclog.Logger, client *http.Client) Requests {
	req := Requests{
		Logger: logger,
		Client: client,
	}
	req.Retryable.MaxAttempts = 10
	req.Retryable.MaxDuration = 500 * time.Millisecond
	req.Retryable.RetryDelay = 5 * time.Minute
	return req
}

func (opts Requests) retryDo(ctx context.Context, req *http.Request) (resp *http.Response, err error) {
	started := time.Now()

	retry := opts.Retryable
	retries := retry.MaxAttempts
	count := 0
	for time.Since(started) < retry.MaxDuration {
		resp, err = opts.Client.Do(req)
		if err != nil && !event.IsErrorRetryable(err) {
			return
		}
		if err == nil {
			// if this request looks like a normal, non-retryable response
			// then just return it without attempting a retry
			if !event.IsHTTPStatusRetryable(resp.StatusCode) {
				return
			}
			// make sure we read all (if any) content and close the response stream as to not leak resources
			if resp.Body != nil {
				ioutil.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}
		if retry.RetryDelay > 0 {
			remaining := math.Min(float64(retry.MaxDuration-time.Since(started)), float64(retry.RetryDelay))
			select {
			case <-ctx.Done():
				return nil, context.Canceled
			case <-time.After(time.Duration(remaining)):
			}
		}
		retries--
		if retries <= 0 {
			return
		}
		count++
		opts.Logger.Info("request failed, will retry", "count", count, "url", req.URL.String())
	}
	return
}

// Do makes an http request. It preserves both request and response body for logging purposes.
// Returns logError function that logs the passed error together with request and response body for easier debugging.
func (opts Requests) Do(ctx context.Context, reqDef *http.Request) (resp *http.Response, logErrorWithRequest func(error) error, rerr error) {
	logger := opts.Logger
	u := reqDef.URL.String()

	req, reqBody, err := requestExtractBody(reqDef)
	if err != nil {
		rerr = err
		return
	}
	req.Header.Set("Accept", "application/json")

	req = req.WithContext(ctx)
	if opts.Retryable.MaxAttempts == 0 {
		resp, err = opts.Client.Do(req)
	} else {
		resp, err = opts.retryDo(ctx, req)
	}
	if err != nil {
		rerr = err
		return
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		rerr = err
		return
	}
	err = resp.Body.Close()
	if err != nil {
		rerr = err
		return
	}
	resp.Body = ioutil.NopCloser(bytes.NewReader(respBody))
	logErrorWithRequest = func(err error) error {
		logger.Debug("error processing response", "err", err.Error(), "url", u, "response_code", resp.StatusCode, "request_body", string(reqBody), "response_body", string(respBody))
		return fmt.Errorf("request failed url: %v err: %v", u, err)
	}
	return
}

func requestExtractBody(req *http.Request) (res *http.Request, reqBody []byte, rerr error) {
	var b []byte

	if req.Body != nil {
		var err error
		b, err = ioutil.ReadAll(req.Body)
		if err != nil {
			rerr = err
			return
		}
	}

	res, err := http.NewRequest(req.Method, req.URL.String(), bytes.NewReader(b))
	if err != nil {
		rerr = err
		return
	}
	res.Header = req.Header

	return res, b, nil
}

// JSON makes http request and unmarshals resulting json. Returns errors on StatusCode != 200. Logs request and response body on errors.
func (opts Requests) JSON(
	reqDef *http.Request,
	res interface{}) (resp *http.Response, rerr error) {
	resp, logError, err := opts.Do(context.TODO(), reqDef)
	if err != nil {
		rerr = err
		return
	}
	if resp.StatusCode != 200 {
		rerr = logError(fmt.Errorf(`wanted status code 200, got %v`, resp.StatusCode))
		return
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		rerr = logError(err)
		return
	}
	err = json.Unmarshal(b, &res)
	if err != nil {
		rerr = logError(err)
		return
	}
	return
}
