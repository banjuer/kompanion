package library_test

import (
	"context"
	"testing"

	"github.com/banjuer/kompanion/internal/entity"
	"github.com/banjuer/kompanion/internal/library"
	"github.com/banjuer/kompanion/internal/storage"
	"github.com/banjuer/kompanion/pkg/logger"
)

func TestShelfListBooks(t *testing.T) {
	// сгенерировать 5 книжек
	// создать Shelf
	// вызвать ListBooks
	// проверить что вернулось 5 книжек
}

func TestUpdateBookMetadataPreservesCoverPath(t *testing.T) {
	repo := &fakeBookRepo{
		book: entity.Book{
			ID:        "book-id",
			Title:     "old title",
			CoverPath: "covers/book-id.jpg",
		},
	}
	shelf := library.NewBookShelf(storage.NewMemoryStorage(), repo, logger.New("error"))

	book, err := shelf.UpdateBookMetadata(context.Background(), "book-id", entity.Book{Title: "new title"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if book.CoverPath != "covers/book-id.jpg" {
		t.Fatalf("expected returned cover path to be preserved, got %q", book.CoverPath)
	}
	if repo.updated.CoverPath != "covers/book-id.jpg" {
		t.Fatalf("expected stored cover path to be preserved, got %q", repo.updated.CoverPath)
	}
}

type fakeBookRepo struct {
	book    entity.Book
	updated entity.Book
}

func (r *fakeBookRepo) Store(context.Context, entity.Book) error {
	return nil
}

func (r *fakeBookRepo) List(context.Context, string, string, int, int) ([]entity.Book, error) {
	return nil, nil
}

func (r *fakeBookRepo) Search(context.Context, string, string, string, int, int) ([]entity.Book, error) {
	return nil, nil
}

func (r *fakeBookRepo) Count(context.Context) (int, error) {
	return 0, nil
}

func (r *fakeBookRepo) CountSearch(context.Context, string) (int, error) {
	return 0, nil
}

func (r *fakeBookRepo) GetById(context.Context, string) (entity.Book, error) {
	return r.book, nil
}

func (r *fakeBookRepo) GetByFileHash(context.Context, string) (entity.Book, error) {
	return entity.Book{}, nil
}

func (r *fakeBookRepo) Update(_ context.Context, book entity.Book) error {
	r.updated = book
	return nil
}

func (r *fakeBookRepo) Delete(context.Context, string) error {
	return nil
}
