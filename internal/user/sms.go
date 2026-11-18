package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"
)

type httpCodeSender struct {
	endpoint string
	client   *http.Client
}

func NewCodeSender(endpoint string, transport http.RoundTripper) (*httpCodeSender, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return nil, errors.New("invalid DIANPING_SMS_ENDPOINT: HTTPS URL required")
	}
	return &httpCodeSender{endpoint: endpoint, client: &http.Client{
		Timeout:       5 * time.Second,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (s *httpCodeSender) SendCode(ctx context.Context, phone, code string) error {
	payload, err := json.Marshal(map[string]string{"phone": phone, "code": code})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("invalid SMS request")
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return errors.New("SMS gateway unreachable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.New("SMS gateway rejected request")
	}
	return nil
}
