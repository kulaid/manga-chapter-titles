package mangaplus

import (
	"fmt"

	"github.com/kulaid/manga-chapter-titles/internal/sources"
	"google.golang.org/protobuf/encoding/protowire"
)

// The response is protobuf. Only two nested string fields are needed, so it is
// read with protowire directly rather than by generating code from a .proto the
// project would then have to keep in step with Shueisha's.
//
// Shape, verified against captured responses:
//
//	Response .1                      success
//	  .8                             title_detail_view
//	    .28                          chapter_list_group (repeated)
//	      .2 .3 .4                   chapter lists (repeated)
//	        .3                       name      e.g. "#001"
//	        .4                       sub_title e.g. "Chapter 1: Dog & Chainsaw"
const (
	fieldSuccess         = 1
	fieldTitleDetailView = 8
	fieldChapterGroup    = 28
	fieldChapterName     = 3
	fieldChapterSubTitle = 4
)

// chapterListTags are the fields of a chapter_list_group that hold chapters.
//
// Aidoku's own model reads tags 2 and 4 only, and so misses everything under
// tag 3 — which is where the most recent chapters sit, the ones most worth
// having because Wikipedia lags them.
var chapterListTags = map[protowire.Number]bool{2: true, 3: true, 4: true}

// decodeTitleDetail pulls the chapter titles out of a title_detailV3 response.
//
// Chapters with no sub_title carry no title and are skipped, as are chapters
// whose name is not a number.
func decodeTitleDetail(body []byte) (sources.Titles, error) {
	titles := sources.Titles{}

	success, err := field(body, fieldSuccess)
	if err != nil {
		return nil, err
	}
	if success == nil {
		// An error payload — most often the "Account Banned" one that a missing
		// Session-Token header provokes — carries no success message.
		return titles, nil
	}

	view, err := field(success, fieldTitleDetailView)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return titles, nil
	}

	groups, err := fields(view, fieldChapterGroup)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if err := eachMessage(group, func(tag protowire.Number, chapter []byte) error {
			if !chapterListTags[tag] {
				return nil
			}
			return readChapter(chapter, titles)
		}); err != nil {
			return nil, err
		}
	}
	return titles, nil
}

// readChapter records one chapter's title, if it has both a number and a name.
func readChapter(chapter []byte, titles sources.Titles) error {
	var name, sub string
	err := eachField(chapter, func(tag protowire.Number, typ protowire.Type, val []byte) error {
		if typ != protowire.BytesType {
			return nil
		}
		switch tag {
		case fieldChapterName:
			name = string(val)
		case fieldChapterSubTitle:
			sub = string(val)
		}
		return nil
	})
	if err != nil {
		return err
	}

	num, ok := chapterNumber(name)
	if !ok {
		return nil
	}
	if title := cleanTitle(sub, num); title != "" {
		titles[num] = title
	}
	return nil
}

// field returns the first length-delimited value of tag, or nil when absent.
func field(b []byte, tag protowire.Number) ([]byte, error) {
	var found []byte
	err := eachField(b, func(n protowire.Number, typ protowire.Type, val []byte) error {
		if found == nil && n == tag && typ == protowire.BytesType {
			found = val
		}
		return nil
	})
	return found, err
}

// fields returns every length-delimited value of tag.
func fields(b []byte, tag protowire.Number) ([][]byte, error) {
	var found [][]byte
	err := eachField(b, func(n protowire.Number, typ protowire.Type, val []byte) error {
		if n == tag && typ == protowire.BytesType {
			found = append(found, val)
		}
		return nil
	})
	return found, err
}

// eachMessage calls fn for every length-delimited field of b.
func eachMessage(b []byte, fn func(tag protowire.Number, val []byte) error) error {
	return eachField(b, func(n protowire.Number, typ protowire.Type, val []byte) error {
		if typ != protowire.BytesType {
			return nil
		}
		return fn(n, val)
	})
}

// eachField walks the fields of a protobuf message, handing each to fn. val is
// meaningful only for length-delimited fields.
func eachField(b []byte, fn func(tag protowire.Number, typ protowire.Type, val []byte) error) error {
	for len(b) > 0 {
		tag, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return fmt.Errorf("malformed protobuf tag: %w", protowire.ParseError(n))
		}
		b = b[n:]

		if typ == protowire.BytesType {
			val, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return fmt.Errorf("malformed protobuf field %d: %w", tag, protowire.ParseError(n))
			}
			b = b[n:]
			if err := fn(tag, typ, val); err != nil {
				return err
			}
			continue
		}

		n = protowire.ConsumeFieldValue(tag, typ, b)
		if n < 0 {
			return fmt.Errorf("malformed protobuf field %d: %w", tag, protowire.ParseError(n))
		}
		b = b[n:]
	}
	return nil
}
