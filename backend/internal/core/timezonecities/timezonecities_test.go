package timezonecities_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"ovc/backend/internal/core/timezonecities"
)

const frontendCopy = "../../../../frontend/src/lib/time/timezone-cities.json"

func TestTimezoneCityTablesAreIdentical(t *testing.T) {
	t.Parallel()
	frontend, err := os.ReadFile(frontendCopy)
	if err != nil {
		t.Fatal(err)
	}

	backend := timezonecities.Source()

	if !bytes.Equal(frontend, backend) {
		t.Fatal("the backend and frontend timezone city tables differ")
	}
}

func TestTheCityTableNamesMoscowInBothLanguages(t *testing.T) {
	t.Parallel()
	table, err := timezonecities.Load()
	if err != nil {
		t.Fatal(err)
	}
	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}

	place := table.Describe(moscow, time.Date(2030, time.March, 12, 15, 0, 0, 0, time.UTC))

	if place.City == nil || place.City.English != "Moscow" || place.City.Russian != "Москвы" || place.Offset != "" {
		t.Fatalf("Moscow is described as %+v", place)
	}
}

func TestAZoneMissingFromTheCityTableFallsBackToItsOffset(t *testing.T) {
	t.Parallel()
	table, err := timezonecities.Load()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"Etc/GMT-5": "UTC+05:00", "Etc/GMT+3": "UTC-03:00", "Asia/Tel_Aviv": "UTC+02:00"}

	for name, want := range cases {
		zone, err := time.LoadLocation(name)
		if err != nil {
			t.Fatal(err)
		}

		place := table.Describe(zone, time.Date(2030, time.January, 15, 12, 0, 0, 0, time.UTC))

		if place.City != nil || place.Offset != want {
			t.Errorf("%s is described as %+v, want %s", name, place, want)
		}
	}
}
