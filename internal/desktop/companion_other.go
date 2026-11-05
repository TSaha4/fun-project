//go:build !windows

package desktop

import (
	"context"
	"errors"
)

type Config struct{}

func Run(context.Context, Config) error {
	return errors.New("desktop companion is currently supported on Windows only")
}
