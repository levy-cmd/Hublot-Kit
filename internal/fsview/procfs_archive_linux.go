//go:build linux

package fsview

import (
	"context"
	"errors"
)

// Fails closed until task 7 (mount checks before a tar) replaces this file:
// no directory download can pass meanwhile.
func (p *procFS) CheckArchive(context.Context, string) error {
	return errors.New("contrôle des montages : pas encore implémenté (tâche 7)")
}
