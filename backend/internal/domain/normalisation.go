package domain

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type NormalisationVersion int

const NormalisationVersion1 NormalisationVersion = 1

type NormalisedText string

var ErrNumberInvalid = errors.New("the text is not a number under number parsing version 1")

var punctuationReplacementsV1 = strings.NewReplacer(
	"«", `"`, "»", `"`, "„", `"`, "“", `"`, "”", `"`, "‟", `"`,
	"‘", "'", "’", "'", "‚", "'", "‛", "'", "′", "'", "‹", "'", "›", "'",
	"‐", "-", "‑", "-", "‒", "-", "–", "-", "—", "-", "―", "-", "−", "-",
)

var numberPatternV1 = regexp.MustCompile(`^[+-]?[0-9]+([.,][0-9]+)?$`)

func NormaliseTextV1(text string) NormalisedText {
	folded := norm.NFKC.String(foldCaseV1(norm.NFKC.String(text)))
	replaced := punctuationReplacementsV1.Replace(replaceYoV1(folded))
	return NormalisedText(strings.Join(strings.Fields(replaced), " "))
}

func NormaliseNFC(text string) string {
	return norm.NFC.String(text)
}

func foldCaseV1(text string) string {
	return strings.Map(foldCherokeeToUpper, cases.Fold().String(text))
}

func foldCherokeeToUpper(letter rune) rune {
	if unicode.Is(unicode.Cherokee, letter) {
		return unicode.ToUpper(letter)
	}
	return letter
}

func replaceYoV1(text string) string {
	for strings.Contains(text, "ё") {
		text = norm.NFKC.String(strings.ReplaceAll(text, "ё", "е"))
	}
	return text
}

func ParseNumberV1(text string) (*big.Rat, error) {
	compact := strings.ReplaceAll(string(NormaliseTextV1(text)), " ", "")
	if strings.Contains(compact, ",") && strings.Contains(compact, ".") {
		return nil, ErrNumberInvalid
	}
	if !numberPatternV1.MatchString(compact) {
		return nil, ErrNumberInvalid
	}
	value, parsed := new(big.Rat).SetString(strings.Replace(compact, ",", ".", 1))
	if !parsed {
		return nil, ErrNumberInvalid
	}
	return value, nil
}
