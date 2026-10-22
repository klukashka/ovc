package testhasher_test

import (
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/testhasher"
)

func argon2Subject(t *testing.T) testhasher.Subject {
	t.Helper()
	current, err := core.NewPasswordHasher(core.Argon2Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1}, core.CryptoRandom(), 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	older, err := core.NewPasswordHasher(core.Argon2Params{MemoryKiB: 32, Iterations: 1, Parallelism: 1}, core.CryptoRandom(), 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return testhasher.Subject{Current: current, Older: older, Foreign: testhasher.New(2)}
}

func TestPasswordHashersKeepTheContract(t *testing.T) {
	subjects := map[string]func(t *testing.T) testhasher.Subject{
		"argon2": argon2Subject,
		"fake": func(t *testing.T) testhasher.Subject {
			return testhasher.Subject{Current: testhasher.New(2), Older: testhasher.New(1), Foreign: argon2Subject(t).Current}
		},
	}
	for subjectName, subject := range subjects {
		for scenarioName, scenario := range testhasher.Contract {
			t.Run(subjectName+"/"+scenarioName, func(t *testing.T) {
				scenario(t, subject(t))
			})
		}
	}
}
