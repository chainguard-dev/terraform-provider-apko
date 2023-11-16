package provider

import (
	"context"
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

var longBackoff = wait.Backoff{
	Duration: 5 * time.Second,
	Factor:   2.0,
	Jitter:   1.0,
	Steps:    3,
}

func retry(ctx context.Context, backoff wait.Backoff, f func(context.Context) error) error {
	errs := []error{}
	if err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		err := f(ctx)
		errs = append(errs, err)

		return err == nil, nil
	}); err != nil {
		errs = append(errs, err)

		return errors.Join(errs...)
	}

	return nil
}
