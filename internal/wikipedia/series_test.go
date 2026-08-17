package wikipedia

import "testing"

func TestPickChapterListArticle(t *testing.T) {
	tests := []struct {
		name   string
		series string
		hits   []string
		want   string
	}{
		{
			name:   "exact chapter list article",
			series: "Chainsaw Man",
			hits:   []string{"List of Chainsaw Man chapters", "Chainsaw Man", "Power (Chainsaw Man)"},
			want:   "List of Chainsaw Man chapters",
		},
		{
			// Wikipedia's search offers unrelated series when a title has no
			// chapter-list article of its own.
			name:   "rejects an unrelated series' list",
			series: "Solo Leveling",
			hits:   []string{"Solo Leveling", "9th Crunchyroll Anime Awards", "List of Hunter × Hunter chapters"},
			want:   "Solo Leveling",
		},
		{
			name:   "no confident match returns nothing",
			series: "Some Obscure Webtoon",
			hits:   []string{"List of Hunter × Hunter chapters", "Webtoon"},
			want:   "",
		},
		{
			name:   "does not confuse a prefix of another series",
			series: "Monster",
			hits:   []string{"List of Monster Musume chapters", "List of Monster Rancher episodes"},
			want:   "",
		},
		{
			name:   "ignores punctuation and stylised characters",
			series: "Hunter x Hunter",
			hits:   []string{"List of Hunter × Hunter chapters"},
			want:   "List of Hunter × Hunter chapters",
		},
		{
			name:   "empty series",
			series: "",
			hits:   []string{"List of Berserk chapters"},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PickChapterListArticle(tt.series, tt.hits); got != tt.want {
				t.Errorf("PickChapterListArticle(%q) = %q, want %q", tt.series, got, tt.want)
			}
		})
	}
}

func TestFindChapterListLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "main template",
			in:   "==Manga==\n{{Main|List of Berserk chapters}}\nSome prose.",
			want: "List of Berserk chapters",
		},
		{
			name: "piped wikilink",
			in:   "See the [[List of Oshi no Ko chapters|chapter list]] for details.",
			want: "List of Oshi no Ko chapters",
		},
		{
			// The Dandadan article names its chapter list only in the infobox.
			name: "infobox volume_list parameter",
			in:   "{{Infobox animanga/Print\n| volume_list  = List of Dandadan chapters\n}}",
			want: "List of Dandadan chapters",
		},
		{
			// {{See also}} hatnotes escape the pipe as {{!}} and italicise the
			// display title.
			name: "see also hatnote with pipe escape",
			in:   "{{See also|List of Dandadan chapters{{!}}List of ''Dandadan'' chapters}}",
			want: "List of Dandadan chapters",
		},
		{
			name: "no link",
			in:   "This article has no chapter list link.",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FindChapterListLink(tt.in); got != tt.want {
				t.Errorf("FindChapterListLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArticleFromRef(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "article path", in: "https://en.wikipedia.org/wiki/List_of_Berserk_chapters", want: "List of Berserk chapters"},
		{
			name: "percent-encoded title",
			in:   "https://en.wikipedia.org/wiki/List_of_Frieren%3A_Beyond_Journey%27s_End_chapters",
			want: "List of Frieren: Beyond Journey's End chapters",
		},
		{name: "index.php form", in: "https://en.wikipedia.org/w/index.php?title=List of Berserk chapters", want: "List of Berserk chapters"},
		{name: "url with fragment", in: "https://en.wikipedia.org/wiki/List_of_Berserk_chapters#Volumes", want: "List of Berserk chapters"},
		{name: "bare article title", in: "List of Berserk chapters", want: "List of Berserk chapters"},
		{name: "empty", in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ArticleFromRef(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ArticleFromRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSeriesNameFromArticle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"List of Chainsaw Man chapters", "Chainsaw Man"},
		{"List of 7 Seeds volumes", "7 Seeds"},
		{"Dandadan", "Dandadan"},
	}
	for _, tt := range tests {
		if got := SeriesNameFromArticle(tt.in); got != tt.want {
			t.Errorf("SeriesNameFromArticle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hunter × Hunter", "hunterhunter"},
		{"Hunter x Hunter", "hunterhunter"},
		{"Kaguya-sama: Love Is War", "kaguyasamaloveiswar"},
		{"Kaguya-sama: Love is War", "kaguyasamaloveiswar"},
		{"X", "x"}, // a title that is only "x" must not vanish
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeName(tt.in); got != tt.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
