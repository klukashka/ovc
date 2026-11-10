package domain_test

import (
	"errors"
	"math/big"
	"testing"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"ovc/backend/internal/domain"
)

type normalisationCase struct {
	input string
	want  domain.NormalisedText
}

func assertNormalisesTo(t *testing.T, cases []normalisationCase) {
	t.Helper()
	for _, testCase := range cases {
		got := domain.NormaliseTextV1(testCase.input)

		if got != testCase.want {
			t.Errorf("NormaliseTextV1(%+q) = %+q, want %+q", testCase.input, got, testCase.want)
		}
	}
}

func TestTextNormalisationV1(t *testing.T) {
	t.Run("compatibility composition", func(t *testing.T) {
		cases := []normalisationCase{
			{"\ufb01", "fi"},
			{"\uff21\uff22\uff23", "abc"},
			{"\u2460", "1"},
			{"е\u0308", "е"},
			{"x\u00b2", "x2"},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("case folding", func(t *testing.T) {
		cases := []normalisationCase{
			{"ПРИВЕТ Мир", "привет мир"},
			{"Straße", "strasse"},
			{"ΣΑΣ ς", "σασ σ"},
			{"\u1e9e", "ss"},
			{"\u13a0\uab70", "\u13a0\u13a0"},
			{"\u13f8\u13f0", "\u13f0\u13f0"},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("composition after case folding", func(t *testing.T) {
		cases := []normalisationCase{
			{"\u01f0", "\u01f0"},
			{"\u1e96", "\u1e96"},
			{"\u1fb7", "\u1fb6ι"},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("yo becomes ye", func(t *testing.T) {
		cases := []normalisationCase{
			{"ёж", "еж"},
			{"ЁЛКА", "елка"},
			{"Е\u0308", "е"},
			{"\u0451\u0308", "е"},
			{"\u0451\u0308\u0308", "е"},
			{"\u0451\u0300", "\u0450"},
			{"\u0451\u0306", "\u04d7"},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("quotes", func(t *testing.T) {
		cases := []normalisationCase{
			{"«a»", `"a"`},
			{"„a“", `"a"`},
			{"”a‟", `"a"`},
			{"‘a’", "'a'"},
			{"‚a‛", "'a'"},
			{"′a‹›", "'a''"},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("dashes", func(t *testing.T) {
		cases := []normalisationCase{
			{"a\u2010b\u2011c", "a-b-c"},
			{"a\u2012b\u2013c", "a-b-c"},
			{"a—b\u2015c\u2212d", "a-b-c-d"},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("whitespace", func(t *testing.T) {
		cases := []normalisationCase{
			{"  a \t\n b  ", "a b"},
			{"a\u00a0\u202f\u2009b", "a b"},
			{"\u3000a\u2028b\u0085", "a b"},
			{" \t\n ", ""},
			{"", ""},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("russian answers", func(t *testing.T) {
		cases := []normalisationCase{
			{"«Ёлка» — ЁЖ", `"елка" - еж`},
			{"  Пётр   Первый ", "петр первый"},
			{"Ростов\u2011на\u2011Дону", "ростов-на-дону"},
		}

		assertNormalisesTo(t, cases)
	})

	t.Run("idempotence", func(t *testing.T) {
		inputs := []string{
			"«Ёлка» — ЁЖ",
			"Straße \ufb01 \u1fb7 \u01f0 \u0130",
			"\u00a8 е \u0308",
			"ё\u0300",
			"ё\u0306",
			"ё\u0308",
			"ё\u0308\u0308\u0308",
			"Ё\u0323\u0308",
			" \u0308",
			"\u2028\u0301a",
			"\u13a0\uab70\u13f8",
			"е\u0301\u0308",
			"\u0451\u0344",
		}

		for _, input := range inputs {
			once := domain.NormaliseTextV1(input)

			twice := domain.NormaliseTextV1(string(once))

			if twice != once {
				t.Errorf("NormaliseTextV1(%+q) = %+q, but normalising that again gives %+q", input, once, twice)
			}
		}
	})
}

func TestTextNormalisationV1UnicodeTablesArePinned(t *testing.T) {
	pinned := "17.0.0"

	normalisationTables, caseTables := norm.Version, unicode.Version

	if normalisationTables != pinned || caseTables != pinned {
		t.Errorf("Unicode tables are %s (normalisation) and %s (case), want %s: a change can alter stored normalised forms and needs a new rules version or an owner decision", normalisationTables, caseTables, pinned)
	}
}

func FuzzTextNormalisationV1IsIdempotent(f *testing.F) {
	for _, seed := range []string{"«Ёлка» — ЁЖ", "ё\u0308", "\u1fb7", "a\u00a0b", "\uab70"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		once := domain.NormaliseTextV1(input)

		twice := domain.NormaliseTextV1(string(once))

		if twice != once {
			t.Errorf("NormaliseTextV1(%+q) = %+q, but normalising that again gives %+q", input, once, twice)
		}
	})
}

func TestNumberParsingV1(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		cases := map[string]*big.Rat{
			"1,5":              big.NewRat(3, 2),
			"-2":               big.NewRat(-2, 1),
			"+3.25":            big.NewRat(13, 4),
			"1 000,5":          big.NewRat(2001, 2),
			"1,000":            big.NewRat(1, 1),
			"1\u00a0000":       big.NewRat(1000, 1),
			"1\u202f000":       big.NewRat(1000, 1),
			"1\u2009000":       big.NewRat(1000, 1),
			"\u2212 7":         big.NewRat(-7, 1),
			"\u20134,25":       big.NewRat(-17, 4),
			"\uff11\uff12":     big.NewRat(12, 1),
			"007":              big.NewRat(7, 1),
			" 0,000001 ":       big.NewRat(1, 1000000),
			"-0":               big.NewRat(0, 1),
			"123456789012,123": big.NewRat(123456789012123, 1000),
		}

		for input, want := range cases {
			got, err := domain.ParseNumberV1(input)

			if err != nil || got.Cmp(want) != 0 {
				t.Errorf("ParseNumberV1(%+q) = %v, %v; want %v", input, got, err, want)
			}
		}
	})

	t.Run("refused", func(t *testing.T) {
		inputs := []string{
			"1,000.5", "1e3", "1E3", "NaN", "nan", "Infinity", "inf", ".5", "5.", "5,", "",
			" ", "1,2,3", "1..2", "--1", "+-1", "0x10", "1/2", "\u0661\u0662", "1_000", "\u0661", "12a", "- -1",
		}

		for _, input := range inputs {
			got, err := domain.ParseNumberV1(input)

			if !errors.Is(err, domain.ErrNumberInvalid) || got != nil {
				t.Errorf("ParseNumberV1(%+q) = %v, %v; want ErrNumberInvalid", input, got, err)
			}
		}
	})
}
