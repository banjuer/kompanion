package bookmeta

import "testing"

func TestParseDoubanBookPage(t *testing.T) {
	page := []byte(`
<html>
<head>
<meta property="og:image" content="https://img.example.com/book.jpg">
<script type="application/ld+json">
{
  "name": "邓小平时代",
  "isbn": "9787108041531",
  "author": [{"name":"傅高义"}],
  "publisher": {"name":"生活·读书·新知三联书店"},
  "datePublished": "2013-01",
  "description": "一部关于邓小平的传记。"
}
</script>
</head>
<body></body>
</html>`)

	result, coverURL, err := parseDoubanBookPage(page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	book := result.Book
	if book.Title != "邓小平时代" {
		t.Fatalf("expected title from json-ld, got %q", book.Title)
	}
	if book.Author != "傅高义" {
		t.Fatalf("expected author from json-ld, got %q", book.Author)
	}
	if book.Publisher != "生活·读书·新知三联书店" {
		t.Fatalf("expected publisher from json-ld, got %q", book.Publisher)
	}
	if book.Year != 2013 {
		t.Fatalf("expected year 2013, got %d", book.Year)
	}
	if book.ISBN != "9787108041531" {
		t.Fatalf("expected normalized isbn, got %q", book.ISBN)
	}
	if book.Description != "一部关于邓小平的传记。" {
		t.Fatalf("expected description, got %q", book.Description)
	}
	if coverURL != "https://img.example.com/book.jpg" {
		t.Fatalf("expected cover url, got %q", coverURL)
	}
}

func TestParseDoubanBookPageFallsBackToInfoBlock(t *testing.T) {
	page := []byte(`
<html>
<body>
<h1><span>悉达多</span></h1>
<div id="info">
<span class="pl">作者:</span> <a>赫尔曼·黑塞</a><br/>
<span class="pl">出版社:</span> 上海人民出版社<br/>
<span class="pl">出版年:</span> 2012-9<br/>
<span class="pl">ISBN:</span> 9787208106087<br/>
</div>
<span property="v:summary"> 成长与求索。 </span>
</body>
</html>`)

	result, _, err := parseDoubanBookPage(page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	book := result.Book
	if book.Title != "悉达多" || book.Author != "赫尔曼·黑塞" || book.Publisher != "上海人民出版社" {
		t.Fatalf("unexpected parsed book: %+v", book)
	}
	if book.Year != 2012 {
		t.Fatalf("expected year 2012, got %d", book.Year)
	}
	if book.Description != "成长与求索。" {
		t.Fatalf("expected cleaned summary, got %q", book.Description)
	}
}
