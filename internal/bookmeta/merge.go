package bookmeta

import "github.com/banjuer/kompanion/internal/entity"

func MergeMissingBookMetadata(book entity.Book, metadata entity.Book) entity.Book {
	if book.Title == "" {
		book.Title = metadata.Title
	}
	if book.Author == "" {
		book.Author = metadata.Author
	}
	if book.Description == "" {
		book.Description = metadata.Description
	}
	if book.Publisher == "" {
		book.Publisher = metadata.Publisher
	}
	if book.Year == 0 {
		book.Year = metadata.Year
	}
	if book.ISBN == "" {
		book.ISBN = metadata.ISBN
	}
	if book.Series == "" {
		book.Series = metadata.Series
	}
	if book.SeriesIndex == nil {
		book.SeriesIndex = metadata.SeriesIndex
	}
	return book
}
