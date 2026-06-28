package bookmeta

import "testing"

func TestParseCookieCloudCookiesFiltersDomain(t *testing.T) {
	data := []byte(`{
  "cookie_data": {
    ".douban.com": [
      {"name":"bid","value":"abc","domain":".douban.com"},
      {"name":"dbcl2","value":"user","domain":"book.douban.com"}
    ],
    ".example.com": [
      {"name":"sid","value":"ignore","domain":".example.com"}
    ]
  }
}`)

	cookies, err := parseCookieCloudCookies(data, "douban.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cookies) != 2 {
		t.Fatalf("expected 2 douban cookies, got %d", len(cookies))
	}
}

func TestDomainMatchesSubdomains(t *testing.T) {
	if !domainMatches("book.douban.com", "douban.com") {
		t.Fatal("expected subdomain to match parent domain")
	}
	if !domainMatches(".douban.com", "book.douban.com") {
		t.Fatal("expected parent cookie domain to match subdomain")
	}
	if domainMatches("example.com", "douban.com") {
		t.Fatal("expected unrelated domain not to match")
	}
}
