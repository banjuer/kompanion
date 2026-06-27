package web

import "testing"

func TestBookMetadataFormToBookAllowsEmptySeriesIndex(t *testing.T) {
	form := bookMetadataForm{
		Title:       "邓小平时代",
		Author:      "【美】傅高义 (Ezra.F.Vogel)",
		ISBN:        "9787108041531",
		SeriesIndex: "",
		Description: "邓小平深刻影响了中国历史和世界历史的走向。",
		Year:        "2013",
		Publisher:   "生活·读书·新知三联书店",
	}

	book, err := form.toBook()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if book.SeriesIndex != nil {
		t.Fatalf("expected nil series index, got %v", book.SeriesIndex)
	}
	if book.Year != 2013 {
		t.Fatalf("expected year 2013, got %d", book.Year)
	}
}

func TestBookMetadataFormToBookParsesSeriesIndex(t *testing.T) {
	form := bookMetadataForm{SeriesIndex: "1.5"}

	book, err := form.toBook()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if book.SeriesIndex == nil || !book.SeriesIndex.Valid || book.SeriesIndex.Decimal.String() != "1.5" {
		t.Fatalf("expected valid series index 1.5, got %v", book.SeriesIndex)
	}
}
