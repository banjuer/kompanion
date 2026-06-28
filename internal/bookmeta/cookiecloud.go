package bookmeta

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type CookieCloudSource struct {
	baseURL  string
	uuid     string
	password string
	client   *http.Client
	ttl      time.Duration

	mu        sync.Mutex
	cachedAt  time.Time
	cachedFor string
	cached    string
}

func NewCookieCloudSource(baseURL, uuid, password string, client *http.Client) *CookieCloudSource {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &CookieCloudSource{
		baseURL:  strings.TrimRight(baseURL, "/"),
		uuid:     uuid,
		password: password,
		client:   client,
		ttl:      30 * time.Minute,
	}
}

func (s *CookieCloudSource) Cookie(ctx context.Context, domain string) (string, error) {
	if s.baseURL == "" || s.uuid == "" || s.password == "" {
		return "", ErrNoCookie
	}

	s.mu.Lock()
	if s.cached != "" && s.cachedFor == domain && time.Since(s.cachedAt) < s.ttl {
		cookie := s.cached
		s.mu.Unlock()
		return cookie, nil
	}
	s.mu.Unlock()

	cookie, err := s.fetchCookie(ctx, domain)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.cached = cookie
	s.cachedFor = domain
	s.cachedAt = time.Now()
	s.mu.Unlock()

	return cookie, nil
}

func (s *CookieCloudSource) fetchCookie(ctx context.Context, domain string) (string, error) {
	body, err := json.Marshal(map[string]string{"password": s.password})
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("%s/get/%s", s.baseURL, url.PathEscape(s.uuid))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("cookiecloud request failed: %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", err
	}

	data, err = s.decryptIfNeeded(data)
	if err != nil {
		return "", err
	}

	cookies, err := parseCookieCloudCookies(data, domain)
	if err != nil {
		return "", err
	}
	if len(cookies) == 0 {
		return "", ErrNoCookie
	}
	sort.Slice(cookies, func(i, j int) bool {
		return cookies[i].Name < cookies[j].Name
	})

	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name == "" {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	if len(parts) == 0 {
		return "", ErrNoCookie
	}
	return strings.Join(parts, "; "), nil
}

func (s *CookieCloudSource) decryptIfNeeded(data []byte) ([]byte, error) {
	var payload struct {
		Encrypted string `json:"encrypted"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Encrypted == "" {
		return data, nil
	}

	return decryptCookieCloudPayload(s.uuid, s.password, payload.Encrypted)
}

type cookieCloudCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

func parseCookieCloudCookies(data []byte, domain string) ([]cookieCloudCookie, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	raw := json.RawMessage(data)
	if v, ok := payload["cookie_data"]; ok {
		raw = v
	}
	var cookieMap map[string][]cookieCloudCookie
	if err := json.Unmarshal(raw, &cookieMap); err == nil {
		return filterCookieMap(cookieMap, domain), nil
	}

	var cookies []cookieCloudCookie
	if err := json.Unmarshal(raw, &cookies); err == nil {
		return filterCookies(cookies, domain), nil
	}

	if _, ok := payload["encrypted"]; ok {
		return nil, fmt.Errorf("cookiecloud response is still encrypted; pass password in request body or upgrade CookieCloud server")
	}
	return nil, fmt.Errorf("cookiecloud response missing cookie_data")
}

func filterCookieMap(cookieMap map[string][]cookieCloudCookie, domain string) []cookieCloudCookie {
	var cookies []cookieCloudCookie
	for mapDomain, domainCookies := range cookieMap {
		for _, cookie := range domainCookies {
			if cookie.Domain == "" {
				cookie.Domain = mapDomain
			}
			if domainMatches(cookie.Domain, domain) || domainMatches(mapDomain, domain) {
				cookies = append(cookies, cookie)
			}
		}
	}
	return cookies
}

func filterCookies(cookies []cookieCloudCookie, domain string) []cookieCloudCookie {
	filtered := make([]cookieCloudCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if domainMatches(cookie.Domain, domain) {
			filtered = append(filtered, cookie)
		}
	}
	return filtered
}

func domainMatches(cookieDomain, target string) bool {
	cookieDomain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookieDomain)), ".")
	target = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(target)), ".")
	return cookieDomain == target || strings.HasSuffix(cookieDomain, "."+target) || strings.HasSuffix(target, "."+cookieDomain)
}

func decryptCookieCloudPayload(uuid, password, encryptedData string) ([]byte, error) {
	hash := md5.Sum([]byte(uuid + "-" + password))
	key := hex.EncodeToString(hash[:])[:16]

	encrypted, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("cookiecloud base64 decode: %w", err)
	}
	if len(encrypted) < 16 || string(encrypted[:8]) != "Salted__" {
		return nil, errors.New("cookiecloud encrypted payload has invalid salt header")
	}

	keyIV, err := cookieCloudBytesToKey([]byte(key), encrypted[8:16], 48)
	if err != nil {
		return nil, err
	}
	return cookieCloudAESDecrypt(encrypted[16:], keyIV[:32], keyIV[32:])
}

func cookieCloudBytesToKey(data, salt []byte, output int) ([]byte, error) {
	if len(salt) != 8 {
		return nil, fmt.Errorf("expected salt of length 8, got %d", len(salt))
	}
	data = append(data, salt...)
	hash := md5.Sum(data)
	key := hash[:]
	finalKey := append([]byte(nil), key...)
	for len(finalKey) < output {
		hash = md5.Sum(append(key, data...))
		key = hash[:]
		finalKey = append(finalKey, key...)
	}
	return finalKey[:output], nil
}

func cookieCloudAESDecrypt(data, key, iv []byte) ([]byte, error) {
	if len(data)%aes.BlockSize != 0 {
		return nil, errors.New("cookiecloud ciphertext is not a multiple of AES block size")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, data)
	return cookieCloudPKCS7Unpad(plain)
}

func cookieCloudPKCS7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data to unpad")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > len(data) {
		return nil, errors.New("invalid cookiecloud padding")
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, errors.New("invalid cookiecloud padding")
		}
	}
	return data[:len(data)-padding], nil
}
