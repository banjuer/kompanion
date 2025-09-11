package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/moroz/uuidv7-go"

	"github.com/banjuer/kompanion/internal/entity"
	"github.com/banjuer/kompanion/internal/storage"
	"github.com/banjuer/kompanion/pkg/logger"
	"github.com/banjuer/kompanion/pkg/metadata"
	"github.com/banjuer/kompanion/pkg/utils"
)

// BookShelf 提供书籍管理操作
type BookShelf struct {
	storage storage.Storage
	repo    BookRepo
	logger  logger.Interface
}

// NewBookShelf 创建BookShelf实例
func NewBookShelf(storage storage.Storage, repo BookRepo, l logger.Interface) *BookShelf {
	return &BookShelf{
		storage: storage,
		repo:    repo,
		logger:  l,
	}
}

func (uc *BookShelf) StoreBook(ctx context.Context, tempFile *os.File, uploadedFilename string) (entity.Book, error) {
	koreaderPartialMD5, err := utils.PartialMD5(tempFile.Name())
	if err != nil {
		return entity.Book{}, fmt.Errorf("BookShelf - StoreBook - PartialMD5: %w", err)
	}
	foundBook, err := uc.repo.GetByFileHash(ctx, koreaderPartialMD5)
	if err == nil {
		return foundBook, entity.ErrBookAlreadyExists
	}

	m, err := metadata.ExtractBookMetadata(tempFile)
	if err != nil {
		return entity.Book{}, fmt.Errorf("BookShelf - StoreBook - exractMetadata: %w", err)
	}
	if m.Format == "" {
		return entity.Book{}, errors.New("BookShelf - StoreBook - unknown file format")
	}

	bookID := uuidv7.Generate()
	createDate := time.Now()
	storagepath := fmt.Sprintf("%s/%s.%s", createDate.Format("2006/01/02"), bookID, m.Format)

	err = uc.storage.Write(ctx, tempFile.Name(), storagepath)
	if err != nil {
		return entity.Book{}, fmt.Errorf("BookShelf - StoreBook - s.storage.Write: %w", err)
	}
	uc.logger.Info("BookShelf - StoreBook - documentID: %s", koreaderPartialMD5)

	coverPath, err := writeCover(ctx, uc.storage, m.Cover, bookID.String())
	if err != nil {
		uc.logger.Error("BookShelf - StoreBook - writeCover: %s", err)
	}

	book := entity.Book{
		ID:         bookID.String(),
		Title:      m.Title,
		Author:     m.Author,
		Publisher:  m.Publisher,
		Year:       0,
		CreatedAt:  createDate,
		UpdatedAt:  createDate,
		ISBN:       m.ISBN,
		DocumentID: koreaderPartialMD5,
		FilePath:   storagepath,
		Format:     m.Format,
		CoverPath:  coverPath,
	}

	// place in database
	err = uc.repo.Store(
		ctx,
		book,
	)
	if err != nil {
		return entity.Book{}, fmt.Errorf("BookShelf - StoreBook - s.repo.Store: %w", err)
	}
	return book, nil
}

// ListBooks -. 从数据库获取书籍列表
func (uc *BookShelf) ListBooks(ctx context.Context,
	sortBy, sortOrder string,
	page, perPage int) (PaginatedBookList, error) {
	books, err := uc.repo.List(ctx, sortBy, sortOrder, page, perPage)
	if err != nil {
		return PaginatedBookList{}, fmt.Errorf("BookShelf - ListBooks - s.repo.List: %w", err)
	}

	totalCount, err := uc.repo.Count(ctx)
	if err != nil {
		return PaginatedBookList{}, fmt.Errorf("BookShelf - ListBooks - s.repo.Count: %w", err)
	}

	pbl := NewPaginatedBookList(
		books,
		perPage,
		page,
		totalCount,
	)

	return pbl, nil
}

// SearchBooks -. 搜索书籍
func (uc *BookShelf) SearchBooks(ctx context.Context,
	query string,
	sortBy, sortOrder string,
	page, perPage int) (PaginatedBookList, error) {
	books, err := uc.repo.Search(ctx, query, sortBy, sortOrder, page, perPage)
	if err != nil {
		return PaginatedBookList{}, fmt.Errorf("BookShelf - SearchBooks - s.repo.Search: %w", err)
	}

	totalCount, err := uc.repo.CountSearch(ctx, query)
	if err != nil {
		return PaginatedBookList{}, fmt.Errorf("BookShelf - SearchBooks - s.repo.CountSearch: %w", err)
	}

	pbl := NewPaginatedBookList(
		books,
		perPage,
		page,
		totalCount,
	)

	return pbl, nil
}

func (uc *BookShelf) ViewBook(ctx context.Context, bookID string) (entity.Book, error) {
	book, err := uc.repo.GetById(ctx, bookID)
	if err != nil {
		return entity.Book{}, fmt.Errorf("BookShelf - GetBook - s.repo.Get: %w", err)
	}

	return book, nil
}

func (uc *BookShelf) UpdateBookMetadata(ctx context.Context, bookID string, metadata entity.Book) (entity.Book, error) {
    book, err := uc.repo.GetById(ctx, bookID)
    if err != nil {
        return entity.Book{}, fmt.Errorf("BookShelf - UpdateBookMetadata - s.repo.Get: %w", err)
    }

    // 创建一个包含所有原始字段的更新对象
    updatedBook := book
    
    // 只更新传入了新值的字段
    if metadata.Title != "" {
        updatedBook.Title = metadata.Title
    }
    if metadata.Author != "" {
        updatedBook.Author = metadata.Author
    }
    if metadata.Publisher != "" {
        updatedBook.Publisher = metadata.Publisher
    }
    if metadata.Year != 0 {
        updatedBook.Year = metadata.Year
    }
    
    // 特殊处理：如果传入的ISBN是空字符串，表示明确要清空ISBN
    if metadata.ISBN != "" || (metadata.ISBN == "" && book.ISBN != "") {
        updatedBook.ISBN = metadata.ISBN
    }
    
    updatedBook.UpdatedAt = time.Now()

    err = uc.repo.Update(ctx, updatedBook)
    if err != nil {
        return entity.Book{}, fmt.Errorf("BookShelf - UpdateBookMetadata - s.repo.Update: %w", err)
    }

    return updatedBook, nil
}

func (uc *BookShelf) DownloadBook(ctx context.Context, bookID string) (entity.Book, *os.File, error) {
	book, err := uc.repo.GetById(ctx, bookID)
	if err != nil {
		return book, nil, fmt.Errorf("BookShelf - DownloadBook - s.repo.Get: %s", err)
	}
	file, err := uc.storage.Read(ctx, book.FilePath)
	if err != nil {
		return book, nil, fmt.Errorf("BookShelf - DownloadBook - s.storage.Read: %s", err)
	}
	return book, file, nil
}

func (uc *BookShelf) ViewCover(ctx context.Context, bookID string) (*os.File, error) {
	book, err := uc.repo.GetById(ctx, bookID)
	if err != nil {
		return nil, fmt.Errorf("BookShelf - ViewCover - s.repo.Get: %s", err)
	}
	if book.CoverPath == "" {
		return nil, fmt.Errorf("BookShelf - ViewCover - no cover")
	}
	file, err := uc.storage.Read(ctx, book.CoverPath)
	if err != nil {
		return nil, fmt.Errorf("BookShelf - ViewCover - s.storage.Read: %s", err)
	}
	return file, nil
}

func writeCover(
	ctx context.Context,
	storage storage.Storage,
	cover []byte,
	bookID string,
) (string, error) {
	if len(cover) == 0 {
		return "", nil
	}
	coverTempFile, err := os.CreateTemp("", "cover")
	if err != nil {
		return "", fmt.Errorf("BookShelf - writeCover - os.CreateTemp: %w", err)
	}
	defer coverTempFile.Close()
	_, err = coverTempFile.Write(cover)
	if err != nil {
		return "", fmt.Errorf("BookShelf - writeCover - coverTempFile.Write: %w", err)
	}

	coverpath := fmt.Sprintf("covers/%s.jpg", bookID)
	err = storage.Write(ctx, coverTempFile.Name(), coverpath)
	if err != nil {
		return "", fmt.Errorf("BookShelf - writeCover - s.storage.Write: %w", err)
	}
	return coverpath, nil
}
