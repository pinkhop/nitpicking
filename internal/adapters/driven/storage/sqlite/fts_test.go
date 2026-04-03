package sqlite

import "testing"

func TestSanitizeFTS5Query(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain word is quoted",
			input: "hello",
			want:  `"hello"`,
		},
		{
			name:  "multiple words become individual quoted tokens with implicit AND",
			input: "hello world",
			want:  `"hello" "world"`,
		},
		{
			name:  "embedded double quote is escaped within its token",
			input: `say "hello"`,
			want:  `"say" """hello"""`,
		},
		{
			name:  "FTS5 boolean operators are neutralized by quoting each token",
			input: "foo AND bar",
			want:  `"foo" "AND" "bar"`,
		},
		{
			name:  "FTS5 column filter is neutralized",
			input: "title:secret",
			want:  `"title:secret"`,
		},
		{
			name:  "FTS5 prefix query is neutralized",
			input: "foo*",
			want:  `"foo*"`,
		},
		{
			name:  "FTS5 NEAR operator is neutralized by quoting each token",
			input: "NEAR(foo bar)",
			want:  `"NEAR(foo" "bar)"`,
		},
		{
			name:  "empty string",
			input: "",
			want:  `""`,
		},
		{
			name:  "extra whitespace is collapsed",
			input: "  JSON   subcommand  ",
			want:  `"JSON" "subcommand"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := sanitizeFTS5Query(tc.input)

			// Then
			if got != tc.want {
				t.Errorf("sanitizeFTS5Query(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
