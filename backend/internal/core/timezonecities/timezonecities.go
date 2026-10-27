package timezonecities

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed timezone-cities.json
var tableSource []byte

type City struct {
	English string `json:"en"`
	Russian string `json:"ru"`
}

type Table struct {
	cities map[string]City
}

func Load() (Table, error) {
	cities := map[string]City{}
	if err := json.Unmarshal(tableSource, &cities); err != nil {
		return Table{}, fmt.Errorf("parse the timezone city table: %w", err)
	}
	return Table{cities: cities}, nil
}

func Source() []byte {
	return append([]byte(nil), tableSource...)
}

type Place struct {
	City   *City
	Offset string
}

func (t Table) Describe(zone *time.Location, at time.Time) Place {
	if city, found := t.cities[zone.String()]; found {
		return Place{City: &city}
	}
	return Place{Offset: Offset(zone, at)}
}

func Offset(zone *time.Location, at time.Time) string {
	_, seconds := at.In(zone).Zone()
	sign := '+'
	if seconds < 0 {
		sign, seconds = '-', -seconds
	}
	return fmt.Sprintf("UTC%c%02d:%02d", sign, seconds/3600, seconds%3600/60)
}
