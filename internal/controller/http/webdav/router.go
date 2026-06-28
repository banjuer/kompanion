package webdav

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/banjuer/kompanion/internal/auth"
	"github.com/banjuer/kompanion/internal/entity"
	"github.com/banjuer/kompanion/internal/library"
	"github.com/banjuer/kompanion/internal/stats"
	"github.com/banjuer/kompanion/pkg/logger"
	"github.com/gin-gonic/gin"
)

type routes struct {
	auth   auth.AuthInterface
	logger logger.Interface
	stats  stats.ReadingStats
	shelf  library.Shelf
}

func NewRouter(
	handler *gin.Engine,
	a auth.AuthInterface,
	l logger.Interface,
	rs stats.ReadingStats,
	shelf library.Shelf,
) {
	// Options
	handler.Use(gin.Logger())
	handler.Use(gin.Recovery())

	r := &routes{auth: a, logger: l, stats: rs, shelf: shelf}
	h := handler.Group("/webdav")
	h.Use(basicAuth(a))
	h.Handle("PROPFIND", "/", r.propfindRoot)
	h.Handle("PROPFIND", "/books", r.propfindBooks)
	h.Handle("PROPFIND", "/books/*filepath", r.propfindBook)
	h.GET("/books/*filepath", r.getBook)
	h.Handle("HEAD", "/books/*filepath", r.headBook)
	h.PUT("/statistics.sqlite3", r.putStatistics)
}

func (r *routes) propfindRoot(c *gin.Context) {
	responses := []webdavResponse{
		collectionResponse("/webdav/", "webdav"),
		collectionResponse("/webdav/books/", "books"),
		fileResponse("/webdav/statistics.sqlite3", "statistics.sqlite3", 0, "application/vnd.sqlite3", time.Now()),
	}
	writeMultistatus(c, responses)
}

func (r *routes) propfindBooks(c *gin.Context) {
	books, err := r.listAllBooks(c)
	if err != nil {
		r.logger.Error(err, "http - webdav - propfindBooks - listAllBooks")
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error listing books"})
		return
	}

	responses := []webdavResponse{collectionResponse("/webdav/books/", "books")}
	for _, book := range books {
		href := "/webdav/books/" + url.PathEscape(webdavBookName(book))
		responses = append(responses, fileResponse(href, webdavBookName(book), 0, book.MimeType(), book.UpdatedAt))
	}
	writeMultistatus(c, responses)
}

func (r *routes) propfindBook(c *gin.Context) {
	if strings.Trim(c.Param("filepath"), "/") == "" {
		r.propfindBooks(c)
		return
	}

	book, err := r.bookFromPath(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "book not found"})
		return
	}
	size := r.bookSize(c, book.ID)
	writeMultistatus(c, []webdavResponse{
		fileResponse("/webdav/books/"+url.PathEscape(webdavBookName(book)), webdavBookName(book), size, book.MimeType(), book.UpdatedAt),
	})
}

func (r *routes) getBook(c *gin.Context) {
	book, file, err := r.downloadBookFromPath(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "book not found"})
		return
	}
	defer file.Close()

	c.Header("Content-Type", book.MimeType())
	c.FileAttachment(file.Name(), book.Filename())
}

func (r *routes) headBook(c *gin.Context) {
	book, file, err := r.downloadBookFromPath(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "book not found"})
		return
	}
	defer file.Close()
	info, _ := file.Stat()
	c.Header("Content-Type", book.MimeType())
	if info != nil {
		c.Header("Content-Length", fmt.Sprintf("%d", info.Size()))
	}
	c.Status(http.StatusOK)
}

func (r *routes) putStatistics(c *gin.Context) {
	device := c.GetString("device_name")
	err := r.stats.Write(c.Request.Context(), c.Request.Body, device)
	if err != nil {
		r.logger.Info("error writing statistics: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error writing statistics"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "statistics updated"})
}

func (r *routes) listAllBooks(c *gin.Context) ([]entity.Book, error) {
	var books []entity.Book
	for page := 1; ; page++ {
		list, err := r.shelf.ListBooks(c.Request.Context(), "title", "asc", page, 100)
		if err != nil {
			return nil, err
		}
		books = append(books, list.Books...)
		if !list.HasNext() {
			return books, nil
		}
	}
}

type webdavResponse struct {
	Href          string
	DisplayName   string
	Collection    bool
	ContentLength int64
	ContentType   string
	LastModified  time.Time
}

func collectionResponse(href, displayName string) webdavResponse {
	return webdavResponse{Href: href, DisplayName: displayName, Collection: true, LastModified: time.Now()}
}

func fileResponse(href, displayName string, contentLength int64, contentType string, lastModified time.Time) webdavResponse {
	return webdavResponse{
		Href:          href,
		DisplayName:   displayName,
		ContentLength: contentLength,
		ContentType:   contentType,
		LastModified:  lastModified,
	}
}

func writeMultistatus(c *gin.Context, responses []webdavResponse) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<D:multistatus xmlns:D="DAV:">`)
	for _, response := range responses {
		b.WriteString(`<D:response>`)
		writeXMLTag(&b, "D:href", response.Href)
		b.WriteString(`<D:propstat><D:prop>`)
		writeXMLTag(&b, "D:displayname", response.DisplayName)
		if response.Collection {
			b.WriteString(`<D:resourcetype><D:collection/></D:resourcetype>`)
		} else {
			b.WriteString(`<D:resourcetype/>`)
			writeXMLTag(&b, "D:getcontentlength", fmt.Sprintf("%d", response.ContentLength))
			if response.ContentType != "" {
				writeXMLTag(&b, "D:getcontenttype", response.ContentType)
			}
		}
		writeXMLTag(&b, "D:getlastmodified", response.LastModified.UTC().Format(http.TimeFormat))
		b.WriteString(`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat>`)
		b.WriteString(`</D:response>`)
	}
	b.WriteString(`</D:multistatus>`)

	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.String(http.StatusMultiStatus, b.String())
}

func writeXMLTag(b *strings.Builder, name, value string) {
	b.WriteByte('<')
	b.WriteString(name)
	b.WriteByte('>')
	_ = xml.EscapeText(b, []byte(value))
	b.WriteString("</")
	b.WriteString(name)
	b.WriteByte('>')
}

func webdavBookName(book entity.Book) string {
	name := strings.TrimSpace(book.Filename())
	if name == "" {
		name = book.ID + "." + bookExtension(book)
	}
	return sanitizePathSegment(name)
}

func sanitizePathSegment(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", `"`, "_", "<", "_", ">", "_", "|", "_")
	return strings.TrimSpace(replacer.Replace(value))
}

func bookExtension(book entity.Book) string {
	ext := path.Ext(book.FilePath)
	if ext == "" {
		return "bin"
	}
	return strings.TrimPrefix(ext, ".")
}

var bookIDPattern = regexp.MustCompile(`([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

func bookIDFromWebDAVPath(value string) string {
	value = strings.TrimPrefix(value, "/")
	unescaped, err := url.PathUnescape(value)
	if err == nil {
		value = unescaped
	}
	if id := bookIDPattern.FindString(value); id != "" {
		return id
	}
	return strings.TrimSuffix(path.Base(value), path.Ext(value))
}

func (r *routes) bookFromPath(c *gin.Context) (entity.Book, error) {
	bookID := bookIDFromWebDAVPath(c.Param("filepath"))
	return r.shelf.ViewBook(c.Request.Context(), bookID)
}

func (r *routes) downloadBookFromPath(c *gin.Context) (entity.Book, *os.File, error) {
	bookID := bookIDFromWebDAVPath(c.Param("filepath"))
	return r.shelf.DownloadBook(c.Request.Context(), bookID)
}

func (r *routes) bookSize(c *gin.Context, bookID string) int64 {
	_, file, err := r.shelf.DownloadBook(c.Request.Context(), bookID)
	if err != nil {
		return 0
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

func basicAuth(auth auth.AuthInterface) gin.HandlerFunc {
	return func(c *gin.Context) {
		username, password, ok := c.Request.BasicAuth()
		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="KOmpanion WebDav"`)
			c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized", "code": 2001})
			c.Abort()
			return
		}
		if !auth.CheckDevicePassword(c.Request.Context(), username, password, true) {
			if !auth.CheckPassword(c.Request.Context(), username, password) {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized", "code": 2001})
				c.Abort()
				return
			}
		}
		c.Set("device_name", username)
		c.Next()
	}
}
