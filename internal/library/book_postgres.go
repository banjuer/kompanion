package library

import (
	"context"
	"fmt"
	"strings"

	"github.com/banjuer/kompanion/internal/entity"
	"github.com/banjuer/kompanion/pkg/postgres"
)

// BookDatabaseRepo -.
type BookDatabaseRepo struct {
	*postgres.Postgres
}

// New -.
func NewBookDatabaseRepo(pg *postgres.Postgres) *BookDatabaseRepo {
	return &BookDatabaseRepo{pg}
}

// Store -. only insert in database
func (bdr *BookDatabaseRepo) Store(ctx context.Context, book entity.Book) error {
	sql := `
		INSERT INTO library_book (id, title, author, publisher, year, created_at, updated_at, isbn, storage_file_path, koreader_partial_md5, storage_cover_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	args := []interface{}{
		book.ID, book.Title, book.Author, book.Publisher, book.Year,
		book.CreatedAt, book.UpdatedAt, book.ISBN, book.FilePath,
		book.DocumentID, book.CoverPath,
	}

	_, err := bdr.Pool.Exec(ctx, sql, args...)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return fmt.Errorf("BookDatabaseRepo - Store - r.Pool.Exec: %w", entity.ErrBookAlreadyExists)
		}
		return fmt.Errorf("BookDatabaseRepo - Store - r.Pool.Exec: %w", err)
	}

	return nil
}

// Update -. only update in database
func (bdr *BookDatabaseRepo) Update(ctx context.Context, book entity.Book) error {
	sql := `
		UPDATE library_book
		SET title = $1,
			author = $2,
			publisher = $3,
			year = $4,
			updated_at = $5,
			isbn = $6
		WHERE id = $7
	`
	args := []interface{}{
		book.Title, book.Author, book.Publisher, book.Year,
		book.UpdatedAt, book.ISBN, book.ID,
	}

	rows, err := bdr.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("BookDatabaseRepo - Update - r.Pool.Exec: %w", err)
	}
	if rows.RowsAffected() == 0 {
		return fmt.Errorf("BookDatabaseRepo - Update - no rows affected")
	}
	return nil
}

// List -. only select from database
func (bdr *BookDatabaseRepo) List(ctx context.Context,
	sortBy, sortOrder string,
	page, perPage int,
) ([]entity.Book, error) {
	switch sortOrder {
	case "asc", "desc":
	default:
		sortOrder = "desc"
	}

	switch sortBy {
	case "title", "author", "publisher", "year", "created_at", "updated_at", "isbn":
	default:
		sortBy = "created_at"
	}

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	// Use limit and offset for pagination, because we don't have a lot of books
	// (yes, it's not the best way to do pagination)
	sql := fmt.Sprintf(`
		SELECT 
			id, title, author, publisher, year, created_at, updated_at, isbn, storage_file_path, koreader_partial_md5, storage_cover_path
		FROM library_book
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, sortBy, sortOrder, perPage, (page-1)*perPage)

	rows, err := bdr.Pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("BookDatabaseRepo - List - r.Pool.Query: %w", err)
	}
	defer rows.Close()

	books := make([]entity.Book, 0)
	for rows.Next() {
		var book entity.Book
		err = rows.Scan(&book.ID, &book.Title, &book.Author, &book.Publisher, &book.Year, &book.CreatedAt, &book.UpdatedAt, &book.ISBN, &book.FilePath, &book.DocumentID, &book.CoverPath)
		if err != nil {
			return nil, fmt.Errorf("BookDatabaseRepo - List - rows.Scan: %w", err)
		}
		books = append(books, book)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("BookDatabaseRepo - List - rows.Err: %w", err)
	}

	return books, nil
}

// Search
func (bdr *BookDatabaseRepo) Search(ctx context.Context, query string, sortBy, sortOrder string, page, perPage int) ([]entity.Book, error) {
	switch sortOrder {
	case "asc", "desc":
	default:
		sortOrder = "desc"
	}

	switch sortBy {
	case "title", "author", "publisher", "year", "created_at", "updated_at", "isbn":
	default:
		sortBy = "created_at"
	}

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	// 准备搜索模式，使用ILIKE进行不区分大小写的模糊匹配
	searchPattern := "%" + query + "%"

	sql := fmt.Sprintf(`
		SELECT 
			id, title, author, publisher, year, created_at, updated_at, isbn, storage_file_path, koreader_partial_md5, storage_cover_path
		FROM library_book
		WHERE title ILIKE $1 
		   OR author ILIKE $1 
		   OR publisher ILIKE $1 
		   OR isbn ILIKE $1
		ORDER BY %s %s
		LIMIT %d OFFSET %d
	`, sortBy, sortOrder, perPage, (page-1)*perPage)

	rows, err := bdr.Pool.Query(ctx, sql, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("BookDatabaseRepo - Search - r.Pool.Query: %w", err)
	}
	defer rows.Close()

	books := make([]entity.Book, 0)
	for rows.Next() {
		var book entity.Book
		err = rows.Scan(&book.ID, &book.Title, &book.Author, &book.Publisher, &book.Year, &book.CreatedAt, &book.UpdatedAt, &book.ISBN, &book.FilePath, &book.DocumentID, &book.CoverPath)
		if err != nil {
			return nil, fmt.Errorf("BookDatabaseRepo - Search - rows.Scan: %w", err)
		}
		books = append(books, book)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("BookDatabaseRepo - Search - rows.Err: %w", err)
	}

	return books, nil
}

// CountSearch -. 计算搜索结果总数
func (bdr *BookDatabaseRepo) CountSearch(ctx context.Context, query string) (int, error) {
	searchPattern := "%" + query + "%"

	sql := `
		SELECT COUNT(*)
		FROM library_book
		WHERE title ILIKE $1 
		   OR author ILIKE $1 
		   OR publisher ILIKE $1 
		   OR isbn ILIKE $1
	`

	var count int
	err := bdr.Pool.QueryRow(ctx, sql, searchPattern).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("BookDatabaseRepo - CountSearch - r.Pool.QueryRow: %w", err)
	}

	return count, nil
}

// Get -. only select from database
func (bdr *BookDatabaseRepo) GetById(ctx context.Context, id string) (entity.Book, error) {
	sql := `
		SELECT id, title, author, publisher, year, created_at, updated_at, isbn, storage_file_path, koreader_partial_md5, storage_cover_path
		FROM library_book
		WHERE id = $1
	`
	args := []interface{}{id}

	row := bdr.Pool.QueryRow(ctx, sql, args...)
	var book entity.Book
	err := row.Scan(&book.ID, &book.Title, &book.Author, &book.Publisher, &book.Year, &book.CreatedAt, &book.UpdatedAt, &book.ISBN, &book.FilePath, &book.DocumentID, &book.CoverPath)
	if err != nil {
		return entity.Book{}, fmt.Errorf("BookDatabaseRepo - Get - r.Pool.QueryRow: %w", err)
	}

	return book, nil
}

// GetByFileHash -. only select from database
func (bdr *BookDatabaseRepo) GetByFileHash(ctx context.Context, fileHash string) (entity.Book, error) {
	sql := `
		SELECT id, title, author, publisher, year, created_at, updated_at, isbn, storage_file_path, koreader_partial_md5, storage_cover_path
		FROM library_book
		WHERE koreader_partial_md5 = $1
	`
	args := []interface{}{fileHash}

	row := bdr.Pool.QueryRow(ctx, sql, args...)
	var book entity.Book
	err := row.Scan(&book.ID, &book.Title, &book.Author, &book.Publisher, &book.Year, &book.CreatedAt, &book.UpdatedAt, &book.ISBN, &book.FilePath, &book.DocumentID, &book.CoverPath)
	if err != nil {
		return entity.Book{}, fmt.Errorf("BookDatabaseRepo - GetByFileHash - r.Pool.QueryRow: %w", err)
	}

	return book, nil
}

// Count -. only select from database
func (bdr *BookDatabaseRepo) Count(ctx context.Context) (int, error) {
	sql := `SELECT count(*) FROM library_book`

	row := bdr.Pool.QueryRow(ctx, sql)
	var count int
	err := row.Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("BookDatabaseRepo - Count - r.Pool.QueryRow: %w", err)
	}

	return count, nil
}
