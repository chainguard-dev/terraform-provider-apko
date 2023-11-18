package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

func TestBackoffSteps(t *testing.T) {
	// Make the backoff behavior very explicit so we don't accidentally have wild numbers.
	mins := []time.Duration{
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
	}

	backoff := longBackoff

	steps := backoff.Steps

	for i := 0; i < steps; i++ {
		m := mins[i]

		if next := backoff.Step(); next < m || next > 2*m {
			t.Errorf("expected backoff.Step() = %v < %v < %v", m, next, 2*m)
		} else {
			t.Logf("backoff.Step() = %v", next)
		}
	}
}

func TestRetry(t *testing.T) {
	shortBackoff := wait.Backoff{
		Duration: 1 * time.Millisecond,
		Factor:   1.0,
		Jitter:   1.0,
		Steps:    3,
	}

	one := fmt.Errorf("the first error")
	two := fmt.Errorf("the second error")
	three := fmt.Errorf("the third error")

	for _, tc := range []struct {
		name string
		errs []error
		pass bool
	}{{
		name: "success",
		errs: []error{one, two, nil},
		pass: true,
	}, {
		name: "failure",
		errs: []error{one, two, three},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			backoff := shortBackoff

			idx := 0
			err := retry(context.Background(), backoff, func(context.Context) error {
				err := tc.errs[idx]
				idx++
				return err
			})

			if tc.pass {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
			} else {
				for _, want := range tc.errs {
					if !errors.Is(err, want) {
						t.Errorf("wanted %v got %v", want, err)
					}
				}
			}
		})
	}
}
