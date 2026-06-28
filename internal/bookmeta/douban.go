package bookmeta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/banjuer/kompanion/internal/entity"
)

type DoubanProvider struct {
	baseURL      string
	cookieDomain string
	cookieSource CookieSource
	client       *http.Client
}

func NewDoubanProvider(cookieSource CookieSource, client *http.Client) *DoubanProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &DoubanProvider{
		baseURL:      "https://book.douban.com",
		cookieDomain: "douban.com",
		cookieSource: cookieSource,
		client:       client,
	}
}

func NewDoubanProviderWithBaseURL(baseURL string, cookieSource CookieSource, client *http.Client) *DoubanProvider {
	provider := NewDoubanProvider(cookieSource, client)
	provider.baseURL = strings.TrimRight(baseURL, "/")
	return provider
}

func (p *DoubanProvider) SetCookieDomain(domain string) {
	if strings.TrimSpace(domain) != "" {
		p.cookieDomain = domain
	}
}

func (p *DoubanProvider) LookupByISBN(ctx context.Context, isbn string) (LookupResult, error) {
	isbn = normalizeISBN(isbn)
	if isbn == "" {
		return LookupResult{}, ErrBookNotFound
	}
	if p.cookieSource == nil {
		return LookupResult{}, ErrNoCookie
	}

	cookie, err := p.cookieSource.Cookie(ctx, p.cookieDomain)
	if err != nil {
		return LookupResult{}, err
	}
	if strings.TrimSpace(cookie) == "" {
		return LookupResult{}, ErrNoCookie
	}

	page, err := p.fetchBookPage(ctx, isbn, cookie)
	if err != nil {
		return LookupResult{}, err
	}

	result, coverURL, err := parseDoubanBookPage(page)
	if err != nil {
		return LookupResult{}, err
	}
	if result.Book.ISBN == "" {
		result.Book.ISBN = isbn
	}
	if coverURL != "" {
		result.Cover = p.fetchCover(ctx, coverURL, cookie)
	}
	return result, nil
}

func (p *DoubanProvider) fetchBookPage(ctx context.Context, isbn, cookie string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/isbn/%s/", strings.TrimRight(p.baseURL, "/"), url.PathEscape(isbn))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrBookNotFound
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("douban rejected request: %s", resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("douban request failed: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte("检测到有异常请求")) || bytes.Contains(body, []byte("sec.douban.com")) {
		return nil, fmt.Errorf("douban requires verification")
	}
	return body, nil
}

func (p *DoubanProvider) fetchCover(ctx context.Context, coverURL, cookie string) []byte {
	if strings.HasPrefix(coverURL, "//") {
		coverURL = "https:" + coverURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", strings.TrimRight(p.baseURL, "/")+"/")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil
	}
	return data
}

func parseDoubanBookPage(page []byte) (LookupResult, string, error) {
	body := string(page)
	book, coverURL := parseDoubanJSONLD(body)

	if book.Title == "" {
		book.Title = firstMatch(body, `(?is)<h1[^>]*>\s*<span[^>]*>(.*?)</span>\s*</h1>`)
	}
	if book.Author == "" {
		book.Author = parseInfoValue(body, "作者")
	}
	if book.Publisher == "" {
		book.Publisher = parseInfoValue(body, "出版社")
	}
	if book.Year == 0 {
		book.Year = parseYear(parseInfoValue(body, "出版年"))
	}
	if book.ISBN == "" {
		book.ISBN = parseInfoValue(body, "ISBN")
	}
	if book.Description == "" {
		book.Description = parseDescription(body)
	}
	if coverURL == "" {
		coverURL = firstMatch(body, `(?is)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	}

	book.Title = cleanText(book.Title)
	book.Author = cleanText(book.Author)
	book.Publisher = cleanText(book.Publisher)
	book.Description = cleanText(book.Description)
	book.ISBN = normalizeISBN(book.ISBN)

	if book.Title == "" && book.Author == "" {
		return LookupResult{}, "", ErrBookNotFound
	}
	return LookupResult{Book: book}, html.UnescapeString(strings.TrimSpace(coverURL)), nil
}

type doubanJSONLD struct {
	Name          string          `json:"name"`
	ISBN          string          `json:"isbn"`
	Description   string          `json:"description"`
	DatePublished string          `json:"datePublished"`
	Image         string          `json:"image"`
	Author        json.RawMessage `json:"author"`
	Publisher     json.RawMessage `json:"publisher"`
}

func parseDoubanJSONLD(body string) (entity.Book, string) {
	raw := firstMatch(body, `(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	if raw == "" {
		return entity.Book{}, ""
	}
	raw = html.UnescapeString(strings.TrimSpace(raw))

	var data doubanJSONLD
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return entity.Book{}, ""
	}

	return entity.Book{
		Title:       data.Name,
		Author:      parseJSONLDNames(data.Author),
		Description: data.Description,
		Publisher:   parseJSONLDNames(data.Publisher),
		Year:        parseYear(data.DatePublished),
		ISBN:        data.ISBN,
	}, data.Image
}

func parseJSONLDNames(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var single struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &single); err == nil && single.Name != "" {
		return single.Name
	}

	var many []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &many); err == nil {
		names := make([]string, 0, len(many))
		for _, item := range many {
			if item.Name != "" {
				names = append(names, item.Name)
			}
		}
		return strings.Join(names, ", ")
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var texts []string
	if err := json.Unmarshal(raw, &texts); err == nil {
		return strings.Join(texts, ", ")
	}
	return ""
}

func parseInfoValue(body, label string) string {
	pattern := fmt.Sprintf(`(?is)<span[^>]*class=["']pl["'][^>]*>\s*%s\s*:?\s*</span>\s*(.*?)(?:<br\s*/?>|</div>)`, regexp.QuoteMeta(label))
	return stripTags(firstMatch(body, pattern))
}

func parseDescription(body string) string {
	if summary := firstMatch(body, `(?is)<span[^>]+property=["']v:summary["'][^>]*>(.*?)</span>`); summary != "" {
		return stripTags(summary)
	}
	intros := regexp.MustCompile(`(?is)<div[^>]+class=["']intro["'][^>]*>(.*?)</div>`).FindAllStringSubmatch(body, -1)
	if len(intros) == 0 {
		return ""
	}
	return stripTags(intros[len(intros)-1][1])
}

func parseYear(value string) int {
	re := regexp.MustCompile(`\d{4}`)
	year := re.FindString(value)
	if year == "" {
		return 0
	}
	n, err := strconv.Atoi(year)
	if err != nil {
		return 0
	}
	return n
}

func firstMatch(body, pattern string) string {
	matches := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func stripTags(value string) string {
	value = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(value, "\n")
	value = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(value, "")
	return html.UnescapeString(value)
}

func cleanText(value string) string {
	lines := strings.Fields(strings.ReplaceAll(value, "\u00a0", " "))
	return strings.Join(lines, " ")
}

func normalizeISBN(isbn string) string {
	isbn = strings.ToUpper(strings.TrimSpace(isbn))
	var b strings.Builder
	for _, r := range isbn {
		if r >= '0' && r <= '9' || r == 'X' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
