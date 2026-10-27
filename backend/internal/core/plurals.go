package core

import "ovc/backend/internal/domain"

type PluralCategory string

const (
	PluralOne   PluralCategory = "one"
	PluralFew   PluralCategory = "few"
	PluralMany  PluralCategory = "many"
	PluralOther PluralCategory = "other"
)

func PluralOf(language domain.Language, count int) PluralCategory {
	if language == domain.LanguageRussian {
		return russianPlural(count)
	}
	if count == 1 {
		return PluralOne
	}
	return PluralOther
}

func russianPlural(count int) PluralCategory {
	if count < 0 {
		count = -count
	}
	lastDigit, lastTwo := count%10, count%100
	switch {
	case lastDigit == 1 && lastTwo != 11:
		return PluralOne
	case lastDigit >= 2 && lastDigit <= 4 && (lastTwo < 12 || lastTwo > 14):
		return PluralFew
	}
	return PluralMany
}

type PluralForms map[PluralCategory]string

func (f PluralForms) For(language domain.Language, count int) string {
	if form, found := f[PluralOf(language, count)]; found {
		return form
	}
	return f[PluralOther]
}
