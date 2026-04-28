package webdav

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/banjuer/kompanion/internal/auth"
	"github.com/banjuer/kompanion/internal/stats"
	"github.com/banjuer/kompanion/pkg/logger"
)

func NewRouter(
	handler *gin.Engine,
	a auth.AuthInterface,
	l logger.Interface,
	rs stats.ReadingStats,
	dirPath string,
) {
	// Options
	handler.Use(gin.Logger())
	handler.Use(gin.Recovery())

	h := handler.Group("/webdav")
	h.Use(basicAuth(a))
	h.Handle("PROPFIND", "/", func(c *gin.Context) {
		file, err := rs.Read(c.Request.Context())
		if err != nil {
			if err == stats.ErrEmptyStats {
				response := `<?xml version="1.0" encoding="UTF-8"?>
				<D:multistatus xmlns:D="DAV:">
					<D:response xmlns:D="DAV:">
						<D:href>/webdav/statistics.sqlite3</D:href>
						<D:propstat>
							<D:prop>
								<D:getcontentlength>0</D:getcontentlength>
								<D:getlastmodified>` + time.Now().Format(time.RFC1123) + `</D:getlastmodified>
								<D:resourcetype/>
							</D:prop>
							<D:status>HTTP/1.1 200 OK</D:status>
						</D:propstat>
					</D:response>
				</D:multistatus>`
				c.Header("Content-Type", "application/xml")
				c.String(http.StatusMultiStatus, response)
				return
			}
			l.Info("error reading statistics", err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error reading statistics"})
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			l.Info("error getting file stats", err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error getting file stats"})
			return
		}

		response := `<?xml version="1.0" encoding="UTF-8"?>
		<D:multistatus xmlns:D="DAV:">
			<D:response xmlns:D="DAV:">
				<D:href>/webdav/statistics.sqlite3</D:href>
				<D:propstat>
					<D:prop>
						<D:getcontentlength>` + fmt.Sprintf("%d", stat.Size()) + `</D:getcontentlength>
						<D:getlastmodified>` + stat.ModTime().Format(time.RFC1123) + `</D:getlastmodified>
						<D:resourcetype/>
					</D:prop>
					<D:status>HTTP/1.1 200 OK</D:status>
				</D:propstat>
			</D:response>
		</D:multistatus>`
		c.Header("Content-Type", "application/xml")
		c.String(http.StatusMultiStatus, response)
	})
	h.GET("/statistics.sqlite3", func(c *gin.Context) {
		file, err := rs.Read(c.Request.Context())
		if err != nil {
			if err == stats.ErrEmptyStats {
				c.JSON(http.StatusNotFound, gin.H{"message": "statistics file not found"})
				return
			}
			l.Info("error reading statistics", err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error reading statistics"})
			return
		}
		defer file.Close()

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Disposition", "attachment; filename=statistics.sqlite3")
		http.ServeFile(c.Writer, c.Request, file.Name())
	})
	h.PUT("/statistics.sqlite3", func(c *gin.Context) {
		device := c.GetString("device_name")
		err := rs.Write(c.Request.Context(), c.Request.Body, device)
		if err != nil {
			l.Info("error writing statistics", err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error writing statistics"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"message": "statistics updated"})
	})
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
			c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized", "code": 2001})
			c.Abort()
			return
		}
		c.Set("device_name", username)
		c.Next()
	}
}
