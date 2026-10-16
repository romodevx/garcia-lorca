package blockers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/garcia-lorca/pkg/blockers"
)

func TestDescribe(t *testing.T) {
	groups := func(n int64) blockers.Count { return blockers.Count{N: n, One: "grupo", Many: "grupos"} }
	subjects := func(n int64) blockers.Count { return blockers.Count{N: n, One: "materia", Many: "materias"} }
	periods := func(n int64) blockers.Count {
		return blockers.Count{N: n, One: "periodo de evaluación", Many: "periodos de evaluación"}
	}
	cases := map[string]struct {
		in   []blockers.Count
		want string
	}{
		"nothing":             {nil, ""},
		"all zero":            {[]blockers.Count{groups(0), subjects(0)}, ""},
		"one, singular":       {[]blockers.Count{groups(1)}, "1 grupo"},
		"one, plural":         {[]blockers.Count{subjects(8)}, "8 materias"},
		"two joined with y":   {[]blockers.Count{groups(2), subjects(8)}, "2 grupos y 8 materias"},
		"zeros are skipped":   {[]blockers.Count{groups(0), subjects(1), periods(3)}, "1 materia y 3 periodos de evaluación"},
		"three, comma then y": {[]blockers.Count{groups(2), subjects(1), periods(1)}, "2 grupos, 1 materia y 1 periodo de evaluación"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, blockers.Describe(tc.in...))
		})
	}
}
