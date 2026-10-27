package core_test

import (
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

func TestRussianAndEnglishPluralRules(t *testing.T) {
	cases := []struct {
		count   int
		russian core.PluralCategory
		english core.PluralCategory
	}{
		{0, core.PluralMany, core.PluralOther},
		{1, core.PluralOne, core.PluralOne},
		{2, core.PluralFew, core.PluralOther},
		{4, core.PluralFew, core.PluralOther},
		{5, core.PluralMany, core.PluralOther},
		{11, core.PluralMany, core.PluralOther},
		{12, core.PluralMany, core.PluralOther},
		{14, core.PluralMany, core.PluralOther},
		{21, core.PluralOne, core.PluralOther},
		{22, core.PluralFew, core.PluralOther},
		{25, core.PluralMany, core.PluralOther},
		{101, core.PluralOne, core.PluralOther},
		{111, core.PluralMany, core.PluralOther},
	}

	for _, testCase := range cases {
		russian, english := core.PluralOf(domain.LanguageRussian, testCase.count), core.PluralOf(domain.LanguageEnglish, testCase.count)

		if russian != testCase.russian || english != testCase.english {
			t.Errorf("%d is %s in Russian and %s in English, want %s and %s", testCase.count, russian, english, testCase.russian, testCase.english)
		}
	}
}

func TestReminderTextsUseThePluralOfTheirHours(t *testing.T) {
	russianDay, _ := core.LessonReminderText(domain.LanguageRussian, 24*time.Hour)
	russianHour, _ := core.LessonReminderText(domain.LanguageRussian, time.Hour)
	englishDay, _ := core.LessonReminderText(domain.LanguageEnglish, 24*time.Hour)

	got := []string{russianDay.SubjectPrefix, russianHour.SubjectPrefix, englishDay.SubjectPrefix}

	want := []string{"Урок через 24 часа", "Урок через 1 час", "Lesson in 24 hours"}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("the subject prefix is %q, want %q", got[index], want[index])
		}
	}
}

func TestLessonStartsAreWrittenInTheRecipientsLanguageAndZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2030, time.March, 12, 15, 0, 0, 0, time.UTC)

	texts := []string{
		core.LessonStartText(domain.LanguageRussian, start, berlin, &core.ZoneLabel{City: "Берлина"}),
		core.LessonStartText(domain.LanguageEnglish, start, berlin, &core.ZoneLabel{Offset: "UTC+01:00"}),
		core.LessonStartText(domain.LanguageEnglish, start, berlin, nil),
	}

	want := []string{"вторник, 12 марта 2030, 16:00 по времени Берлина", "Tuesday, 12 March 2030, 16:00 UTC+01:00", "Tuesday, 12 March 2030, 16:00"}
	for index := range want {
		if texts[index] != want[index] {
			t.Errorf("the start reads %q, want %q", texts[index], want[index])
		}
	}
}
