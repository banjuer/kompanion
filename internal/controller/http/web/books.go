package web

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/banjuer/kompanion/internal/entity"
	"github.com/banjuer/kompanion/internal/library"
	"github.com/banjuer/kompanion/internal/stats"
	syncpkg "github.com/banjuer/kompanion/internal/sync"
	"github.com/banjuer/kompanion/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type booksRoutes struct {
	shelf    library.Shelf
	stats    stats.ReadingStats
	progress syncpkg.Progress
	logger   logger.Interface
}

type bookMetadataForm struct {
	Title       string `form:"title"`
	Author      string `form:"author"`
	Description string `form:"description"`
	Publisher   string `form:"publisher"`
	Year        string `form:"year"`
	Series      string `form:"series"`
	SeriesIndex string `form:"series_index"`
	ISBN        string `form:"isbn"`
}

func (f bookMetadataForm) toBook() (entity.Book, error) {
	var book entity.Book

	book.Title = f.Title
	book.Author = f.Author
	book.Description = f.Description
	book.Publisher = f.Publisher
	book.Series = f.Series
	book.ISBN = f.ISBN

	if year := strings.TrimSpace(f.Year); year != "" {
		parsedYear, err := strconv.Atoi(year)
		if err != nil {
			return entity.Book{}, fmt.Errorf("invalid year: %w", err)
		}
		book.Year = parsedYear
	}

	if seriesIndex := strings.TrimSpace(f.SeriesIndex); seriesIndex != "" {
		parsedSeriesIndex, err := decimal.NewFromString(seriesIndex)
		if err != nil {
			return entity.Book{}, fmt.Errorf("invalid series index: %w", err)
		}
		nullSeriesIndex := decimal.NewNullDecimal(parsedSeriesIndex)
		book.SeriesIndex = &nullSeriesIndex
	}

	return book, nil
}

func newBooksRoutes(handler *gin.RouterGroup, shelf library.Shelf, stats stats.ReadingStats, progress syncpkg.Progress, l logger.Interface) {
	r := &booksRoutes{shelf: shelf, stats: stats, progress: progress, logger: l}

	handler.GET("/", r.listBooks)
	handler.POST("/upload", r.uploadBook)
	handler.GET("/:bookID", r.viewBook)
	handler.POST("/:bookID", r.updateBookMetadata)
	handler.POST("/:bookID/enrich", r.enrichBookMetadata)
	handler.DELETE("/:bookID", r.deleteBook)
	handler.GET("/:bookID/download", r.downloadBook)
	handler.GET("/:bookID/cover", r.viewBookCover)
	handler.POST("/:bookID/cover", r.uploadBookCover)
}

func (r *booksRoutes) listBooks(c *gin.Context) {
	page := 1
	perPage := 10 // Default 10 books per page
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if perPageStr := c.Query("perPage"); perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 {
			// Limit perPage to reasonable values
			if pp <= 100 {
				perPage = pp
			} else {
				perPage = 100
			}
		}
	}

	// 获取搜索查询参数
	query := c.Query("q")

	var books library.PaginatedBookList
	var err error

	// 根据是否有搜索查询来决定调用哪个方法
	if query != "" {
		books, err = r.shelf.SearchBooks(c.Request.Context(), query, "created_at", "desc", page, perPage)
	} else {
		books, err = r.shelf.ListBooks(c.Request.Context(), "created_at", "desc", page, perPage)
	}

	if err != nil {
		c.HTML(500, "error", passStandartContext(c, gin.H{"error": err.Error()}))
		return
	}

	// Fetch progress for each book
	type BookWithProgress struct {
		entity.Book
		Progress int
	}
	booksWithProgress := make([]BookWithProgress, len(books.Books))
	for i, book := range books.Books {
		progress, err := r.progress.Fetch(c.Request.Context(), book.DocumentID)
		if err != nil {
			r.logger.Error(err, "failed to fetch progress for book %s", book.ID)
			progress = entity.Progress{}
		}
		booksWithProgress[i] = BookWithProgress{
			Book:     book,
			Progress: int(progress.Percentage * 100),
		}
	}

	c.HTML(200, "books", passStandartContext(c, gin.H{
		"books": booksWithProgress,
		"query": query, // 传递搜索查询到模板，以便在搜索框中显示
		"pagination": gin.H{
			"currentPage": page,
			"perPage":     perPage,
			"totalPages":  books.TotalPages(),
			"hasNext":     books.HasNext(),
			"hasPrev":     books.HasPrev(),
			"nextPage":    books.Next(),
			"prevPage":    books.Prev(),
			"firstPage":   books.First(),
			"lastPage":    books.Last(),
		},
	}))
}

func (r *booksRoutes) uploadBook(c *gin.Context) {
	// single uploadedBookFile
	uploadedBookFile, err := c.FormFile("book")
	if err != nil {
		r.logger.Error(err, "http - v1 - shelf - uploadBook")
		c.JSON(400, passStandartContext(c, gin.H{"message": "book file is required"}))
		return
	}

	// make by temp files
	tempFile, err := os.CreateTemp("", "")
	if err != nil {
		r.logger.Error(err, "http - v1 - shelf - putBook")
		c.JSON(500, passStandartContext(c, gin.H{"message": "bad request"}))
		return
	}
	filepath := tempFile.Name()
	defer os.Remove(filepath)
	defer tempFile.Close()
	c.SaveUploadedFile(uploadedBookFile, filepath)

	book, err := r.shelf.StoreBook(c.Request.Context(), tempFile, uploadedBookFile.Filename)
	if err != nil && err != entity.ErrBookAlreadyExists {
		r.logger.Error(err, "http - v1 - shelf - putBook")
		c.JSON(500, passStandartContext(c, gin.H{"message": "internal server error"}))
		return
	}
	c.Redirect(302, "/books/"+book.ID)
}

func (r *booksRoutes) downloadBook(c *gin.Context) {
	bookID := c.Param("bookID")

	book, file, err := r.shelf.DownloadBook(c.Request.Context(), bookID)
	if err != nil {
		c.JSON(500, passStandartContext(c, gin.H{"message": "internal server error"}))
		return
	}
	defer file.Close()

	c.Header("Content-Disposition", "attachment; filename="+book.Filename())
	c.Header("Content-Type", "application/octet-stream")
	c.File(file.Name())
}

func (r *booksRoutes) viewBook(c *gin.Context) {
	bookID := c.Param("bookID")

	book, err := r.shelf.ViewBook(c.Request.Context(), bookID)
	if err != nil {
		c.HTML(500, "error", passStandartContext(c, gin.H{"error": err.Error()}))
		return
	}

	bookStats, err := r.stats.GetBookStats(c.Request.Context(), book.DocumentID)
	if err != nil {
		r.logger.Error(err, "failed to get book stats")
		bookStats = &stats.BookStats{} // Use empty stats in case of error
	}

	c.HTML(200, "book", passStandartContext(c, gin.H{
		"book":          book,
		"stats":         bookStats,
		"metadataError": c.Query("metadata_error"),
	}))
}

func (r *booksRoutes) updateBookMetadata(c *gin.Context) {
	bookID := c.Param("bookID")

	var form bookMetadataForm
	if err := c.ShouldBind(&form); err != nil {
		r.logger.Error(err, "http - v1 - shelf - updateBookMetadata")
		// TODO: move to template
		c.JSON(400, passStandartContext(c, gin.H{"message": "invalid request"}))
		return
	}

	metadata, err := form.toBook()
	if err != nil {
		r.logger.Error(err, "http - v1 - shelf - updateBookMetadata")
		// TODO: move to template
		c.JSON(400, passStandartContext(c, gin.H{"message": "invalid request"}))
		return
	}

	book, err := r.shelf.UpdateBookMetadata(c.Request.Context(), bookID, metadata)
	if err != nil {
		r.logger.Error(err, "http - v1 - shelf - updateBookMetadata")
		// TODO: move to template
		c.JSON(500, passStandartContext(c, gin.H{"message": "internal server error"}))
		return
	}

	// TODO: why not redirect?
	c.HTML(200, "book", passStandartContext(c, gin.H{"book": book}))
}

func (r *booksRoutes) enrichBookMetadata(c *gin.Context) {
	bookID := c.Param("bookID")

	isbn := strings.TrimSpace(c.PostForm("isbn"))
	if isbn != "" {
		if _, err := r.shelf.UpdateBookMetadata(c.Request.Context(), bookID, entity.Book{ISBN: isbn}); err != nil {
			r.logger.Error(err, "http - web - books - enrichBookMetadata - updateISBN")
			c.String(500, "failed to update ISBN before fetching metadata")
			return
		}
	}

	_, err := r.shelf.EnrichBookMetadata(c.Request.Context(), bookID)
	if err != nil {
		r.logger.Error(err, "http - web - books - enrichBookMetadata")
		c.Redirect(303, "/books/"+bookID+"?metadata_error="+url.QueryEscape(err.Error()))
		return
	}

	c.Redirect(302, "/books/"+bookID)
}

func (r *booksRoutes) viewBookCover(c *gin.Context) {
	bookID := c.Param("bookID")

	book, err := r.shelf.ViewBook(c.Request.Context(), bookID)
	if err != nil {
		c.JSON(500, passStandartContext(c, gin.H{"message": "internal server error"}))
		return
	}

	cover, err := r.shelf.ViewCover(c.Request.Context(), bookID)

	if err != nil {
		width := 600
		height := 800
		backgroundColor := "#6496FA" // Цвет фона (голубой)
		textColor := "white"         // Цвет текста
		title := book.Title
		subtitle := book.Author
		fontSizeTitle := 48
		fontSizeSubtitle := 24

		svgContent := fmt.Sprintf(`
		<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">
			<rect width="100%%" height="100%%" fill="%s" />
			<text x="50%%" y="40%%" font-family="Arial" font-size="%d" fill="%s" text-anchor="middle">%s</text>
			<text x="50%%" y="55%%" font-family="Arial" font-size="%d" fill="%s" text-anchor="middle">%s</text>
		</svg>
		`, width, height, backgroundColor, fontSizeTitle, textColor, title, fontSizeSubtitle, textColor, subtitle)

		c.Data(200, "image/svg+xml", []byte(svgContent))
		return
	}
	c.File(cover.Name())
}

func (r *booksRoutes) uploadBookCover(c *gin.Context) {
	bookID := c.Param("bookID")

	coverFile, err := c.FormFile("cover")
	if err != nil {
		r.logger.Error(err, "http - web - books - uploadBookCover - missing cover file")
		c.JSON(400, gin.H{"message": "cover file is required"})
		return
	}

	tempFile, err := os.CreateTemp("", "cover-")
	if err != nil {
		r.logger.Error(err, "http - web - books - uploadBookCover - create temp")
		c.JSON(500, gin.H{"message": "internal server error"})
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if err := c.SaveUploadedFile(coverFile, tempFile.Name()); err != nil {
		r.logger.Error(err, "http - web - books - uploadBookCover - save uploaded")
		c.JSON(500, gin.H{"message": "internal server error"})
		return
	}

	_, err = r.shelf.UpdateCover(c.Request.Context(), bookID, tempFile)
	if err != nil {
		r.logger.Error(err, "http - web - books - uploadBookCover - UpdateCover")
		c.JSON(500, gin.H{"message": "internal server error"})
		return
	}

	c.JSON(200, gin.H{"message": "cover updated successfully"})
}

func (r *booksRoutes) deleteBook(c *gin.Context) {
	bookID := c.Param("bookID")

	err := r.shelf.DeleteBook(c.Request.Context(), bookID)
	if err != nil {
		r.logger.Error(err, "http - v1 - shelf - deleteBook")
		c.JSON(500, passStandartContext(c, gin.H{"message": "internal server error"}))
		return
	}

	c.Redirect(302, "/books")
}
