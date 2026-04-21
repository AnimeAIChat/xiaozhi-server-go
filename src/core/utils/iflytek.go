package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func BuildIFlytekAuthURL(baseURL, apiKey, apiSecret string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("missing iFlytek api_key")
	}
	if apiSecret == "" {
		return "", fmt.Errorf("missing iFlytek api_secret")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse iFlytek url failed: %w", err)
	}

	requestPath := parsedURL.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}

	dateValue := time.Now().UTC().Format(http.TimeFormat)
	signatureOrigin := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", parsedURL.Host, dateValue, requestPath)

	mac := hmac.New(sha256.New, []byte(apiSecret))
	if _, err := mac.Write([]byte(signatureOrigin)); err != nil {
		return "", fmt.Errorf("build iFlytek signature failed: %w", err)
	}
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	authorizationOrigin := fmt.Sprintf(
		`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`,
		apiKey,
		signature,
	)

	query := parsedURL.Query()
	query.Set("authorization", base64.StdEncoding.EncodeToString([]byte(authorizationOrigin)))
	query.Set("date", dateValue)
	query.Set("host", parsedURL.Host)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}
