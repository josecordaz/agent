package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

func (s *Integration) makeRequest(query string, res interface{}) error {
	s.requestConcurrencyChan <- true
	defer func() {
		<-s.requestConcurrencyChan
	}()
	return s.makeRequestRetry(query, res, 0)
}

const maxGeneralRetries = 2

func (s *Integration) makeRequestRetry(query string, res interface{}, generalRetry int) error {
	isRetryable, err := s.makeRequestRetryThrottled(query, res, 0)
	if err != nil {
		if !isRetryable {
			return err
		}
		if generalRetry >= maxGeneralRetries {
			return fmt.Errorf(`can't retry request too many retries, err: %v`, err)
		}
		time.Sleep(time.Duration(1+generalRetry) * time.Minute)
		return s.makeRequestRetry(query, res, generalRetry+1)
	}
	return nil
}

const maxThrottledRetries = 3

func (s *Integration) makeRequestRetryThrottled(query string, res interface{}, retryThrottled int) (isErrorRetryable bool, rerr error) {

	data := map[string]string{
		"query": query,
	}

	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest("POST", s.config.APIURL, bytes.NewReader(b))
	if err != nil {
		rerr = err
		return
	}
	req.Header.Add("Authorization", "bearer "+s.config.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		rerr = err
		isErrorRetryable = true
		return
	}
	defer resp.Body.Close()

	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		rerr = err
		isErrorRetryable = true
		return
	}

	rateLimited := func() (isErrorRetryable bool, rerr error) {
		if retryThrottled >= maxThrottledRetries {
			s.logger.Info("api request failed", "body", string(b))
			rerr = fmt.Errorf(`resp resp.StatusCode != 200, got %v, can't retry, too many retries already`, resp.StatusCode)
			return
		}

		s.logger.Warn("api request failed due to throttling, will sleep for 30m and retry, this should only happen if hourly quota is used up, check here (https://developer.github.com/v4/guides/resource-limitations/#returning-a-calls-rate-limit-status)", "body", string(b), "retryThrottled", retryThrottled)
		time.Sleep(30 * time.Minute)

		return s.makeRequestRetryThrottled(query, res, retryThrottled+1)

	}

	// check if there were errors returned first

	if resp.StatusCode != 200 {

		if resp.StatusCode == 403 && strings.Contains(string(b), "wait") {
			return rateLimited()
		}

		var errRes struct {
			Message string `json:"message"`
		}

		if resp.StatusCode == 401 {
			json.Unmarshal(b, &errRes)
			if errRes.Message != "" {
				rerr = fmt.Errorf(`github request failed with status code %v: %v`, resp.StatusCode, errRes.Message)
				isErrorRetryable = false
				return
			}
		}

		s.logger.Info("api request failed", "body", string(b), "code", resp.StatusCode)
		rerr = fmt.Errorf(`github request failed with status code %v`, resp.StatusCode)
		isErrorRetryable = false
		return
	}

	var errRes struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}

	json.Unmarshal(b, &errRes)
	if len(errRes.Errors) != 0 {
		s.logger.Info("api request got errors returned in json", "body", string(b))

		err1 := errRes.Errors[0]
		// "{"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}"
		if err1.Type == "RATE_LIMITED" {
			return rateLimited()
		}

		rerr = fmt.Errorf("api request failed: type: %v message %v", err1.Type, err1.Message)
		return
	}

	//s.logger.Info("response body", string(b))

	err = json.Unmarshal(b, &res)
	if err != nil {
		rerr = err
		return
	}

	return
}