package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Level is the education level a pupil is placed in: a reference to a level
// the school defined in its catalog (config), with the name the school gave it.
//
// It used to be a fixed SEP list (maternal … preparatoria), and a school that
// named its level "Preescolar", or had one the list never did, could create it
// in Configuración and then not place a single pupil in it. Which levels exist
// is the school's call, made in config; students only records which one a
// pupil is in (ADR 0014).
type Level struct {
	id   string
	name string
}

// NewLevel builds a level from a catalog entry. Both the config id and the
// name are required: the id is what the pupil points at, the name is what
// every list shows without calling config for each row.
func NewLevel(id, name string) (Level, error) {
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	if id == "" {
		return Level{}, glerrors.New(glerrors.KindInvalid, "a level is required")
	}
	if name == "" {
		return Level{}, glerrors.New(glerrors.KindInvalid, "a level needs a name")
	}
	return Level{id: id, name: name}, nil
}

// RestoreLevel rebuilds a stored level without judging it. A pupil placed
// before the catalog went free may carry a name and no id (its old key matched
// no config level); it stays readable as it is.
func RestoreLevel(id, name string) Level { return Level{id: id, name: name} }

// ID is the config level's id ("" only for an unlinked legacy pupil).
func (l Level) ID() string { return l.id }

// Name is the level as the school wrote it ("Preescolar").
func (l Level) Name() string { return l.name }

// IsZero reports whether no level was given at all.
func (l Level) IsZero() bool { return l.id == "" && l.name == "" }
