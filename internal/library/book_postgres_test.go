package library_test

import (
	"context"
	"testing"
	"time"

	"github.com/banjuer/kompanion/internal/entity"
	"github.com/banjuer/kompanion/internal/library"
	"github.com/banjuer/kompanion/pkg/postgres"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/shopspring/decimal"
)

func TestBookDatabaseRepoCreate(t *testing.T) {
	seriesIndex := decimal.NewNullDecimal(decimal.RequireFromString("1.5"))
	book := entity.Book{
		ID:          "1",
		Title:       "title",
		Author:      "author",
		Description: "A test book description",
		Publisher:   "publisher",
		Year:        2021,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ISBN:        "isbn",
		FilePath:    "file_path",
		DocumentID:  "document_id",
		CoverPath:   "cover_path",
		Series:      "Test Series",
		SeriesIndex: &seriesIndex,
	}

	mock, bdr := setupTestBookDatabaseRepo()
	defer mock.Close()

	mock.ExpectExec("INSERT INTO library_book").
		WithArgs(book.ID, book.Title, book.Author, book.Publisher, book.Year, book.CreatedAt, book.UpdatedAt, book.ISBN, book.FilePath, book.DocumentID, book.CoverPath, book.Series, book.SeriesIndex, book.Description).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err := bdr.Store(context.Background(), book)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBookDatabaseRepoUpdateStoresCoverPath(t *testing.T) {
	seriesIndex := decimal.NewNullDecimal(decimal.RequireFromString("1.5"))
	book := entity.Book{
		ID:          "1",
		Title:       "title",
		Author:      "author",
		Description: "A test book description",
		Publisher:   "publisher",
		Year:        2021,
		UpdatedAt:   time.Now(),
		ISBN:        "isbn",
		CoverPath:   "covers/1.jpg",
		Series:      "Test Series",
		SeriesIndex: &seriesIndex,
	}

	mock, bdr := setupTestBookDatabaseRepo()
	defer mock.Close()

	mock.ExpectExec("UPDATE library_book").
		WithArgs(book.Title, book.Author, book.Publisher, book.Year, book.UpdatedAt, book.ISBN, book.Series, book.SeriesIndex, book.Description, book.CoverPath, book.ID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := bdr.Update(context.Background(), book)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBookDatabaseRepoGetById(t *testing.T) {
	seriesIndex := decimal.NewNullDecimal(decimal.RequireFromString("2"))
	book := entity.Book{
		ID:          "1",
		Title:       "title",
		Author:      "author",
		Description: "A test book description",
		Publisher:   "publisher",
		Year:        2021,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ISBN:        "isbn",
		FilePath:    "file_path",
		DocumentID:  "document_id",
		CoverPath:   "cover_path",
		Series:      "Test Series",
		SeriesIndex: &seriesIndex,
	}

	mock, bdr := setupTestBookDatabaseRepo()
	defer mock.Close()

	rows := pgxmock.NewRows([]string{"id", "title", "author", "publisher", "year", "created_at", "updated_at", "isbn", "file_path", "file_hash", "cover_path", "series", "series_index", "summary"}).
		AddRow(book.ID, book.Title, book.Author, book.Publisher, book.Year, book.CreatedAt, book.UpdatedAt, book.ISBN, book.FilePath, book.DocumentID, book.CoverPath, book.Series, "2", book.Description)

	mock.ExpectQuery("SELECT (.+) FROM library_book").
		WithArgs(book.ID).
		WillReturnRows(rows)

	result, err := bdr.GetById(context.Background(), book.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.DocumentID != book.DocumentID {
		t.Errorf("expected DocumentID %v, got %v", book.DocumentID, result.DocumentID)
	}
}

func TestBookDatabaseRepoGetByFileHash(t *testing.T) {
	seriesIndex := decimal.NewNullDecimal(decimal.RequireFromString("1"))
	book := entity.Book{
		ID:          "1",
		Title:       "title",
		Author:      "author",
		Description: "A test book description",
		Publisher:   "publisher",
		Year:        2021,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ISBN:        "isbn",
		FilePath:    "file_path",
		DocumentID:  "document_id",
		CoverPath:   "cover_path",
		Series:      "Test Series",
		SeriesIndex: &seriesIndex,
	}

	mock, bdr := setupTestBookDatabaseRepo()
	defer mock.Close()

	rows := pgxmock.NewRows([]string{"id", "title", "author", "publisher", "year", "created_at", "updated_at", "isbn", "file_path", "file_hash", "cover_path", "series", "series_index", "summary"}).
		AddRow(book.ID, book.Title, book.Author, book.Publisher, book.Year, book.CreatedAt, book.UpdatedAt, book.ISBN, book.FilePath, book.DocumentID, book.CoverPath, book.Series, "1", book.Description)

	mock.ExpectQuery("SELECT (.+) FROM library_book").
		WithArgs(book.DocumentID).
		WillReturnRows(rows)

	result, err := bdr.GetByFileHash(context.Background(), book.DocumentID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.DocumentID != book.DocumentID {
		t.Errorf("expected DocumentID %v, got %v", book.DocumentID, result.DocumentID)
	}
}

func TestBookDatabaseRepoList(t *testing.T) {
	seriesIndex := decimal.NewNullDecimal(decimal.RequireFromString("3.5"))
	book := entity.Book{
		ID:          "1",
		Title:       "title",
		Author:      "author",
		Description: "A test book description",
		Publisher:   "publisher",
		Year:        2021,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ISBN:        "isbn",
		FilePath:    "file_path",
		DocumentID:  "document_id",
		CoverPath:   "cover_path",
		Series:      "Test Series",
		SeriesIndex: &seriesIndex,
	}

	mock, bdr := setupTestBookDatabaseRepo()
	defer mock.Close()

	rows := pgxmock.NewRows([]string{"id", "title", "author", "publisher", "year", "created_at", "updated_at", "isbn", "file_path", "file_hash", "cover_path", "series", "series_index", "summary"}).
		AddRow(book.ID, book.Title, book.Author, book.Publisher, book.Year, book.CreatedAt, book.UpdatedAt, book.ISBN, book.FilePath, book.DocumentID, book.CoverPath, book.Series, "3.5", book.Description)

	mock.ExpectQuery("SELECT (.+) FROM library_book").
		WillReturnRows(rows)

	results, err := bdr.List(context.Background(), "created_at", "desc", 1, 10)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %v", len(results))
		return
	}

	if results[0].DocumentID != book.DocumentID {
		t.Errorf("expected DocumentID %v, got %v", book.DocumentID, results[0].DocumentID)
	}
}

func setupTestBookDatabaseRepo() (pgxmock.PgxPoolIface, *library.BookDatabaseRepo) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		panic(err)
	}

	pg := postgres.Mock(mock)
	bdr := library.NewBookDatabaseRepo(pg)

	return mock, bdr
}
