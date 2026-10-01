package receipt

import (
	"context"
	"fmt"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// StatementReader reads a student's ledger from finance (T7): the office's
// view or the family's.
type StatementReader interface {
	GetStatement(ctx context.Context, studentID string) (client.Statement, error)
	GetFamilyStatement(ctx context.Context, studentID string) (client.Statement, error)
}

// StatementDoc is everything the printed account statement says.
type StatementDoc struct {
	School      client.SchoolProfile
	Pupil       string
	PupilDetail string
	Statement   client.Statement
}

// StatementBuilder assembles a statement document from finance's ledger, the
// pupil and the school's details. It computes nothing: the ledger is finance's.
type StatementBuilder struct {
	statements StatementReader
	students   StudentReader
	profiles   ProfileReader
}

// NewStatementBuilder wires a StatementBuilder.
func NewStatementBuilder(statements StatementReader, students StudentReader, profiles ProfileReader) *StatementBuilder {
	return &StatementBuilder{statements: statements, students: students, profiles: profiles}
}

// Ledger is the student's ledger as the office (family=false) or the family
// sees it.
func (b *StatementBuilder) Ledger(ctx context.Context, studentID string, family bool) (client.Statement, error) {
	if family {
		return b.statements.GetFamilyStatement(ctx, studentID)
	}
	return b.statements.GetStatement(ctx, studentID)
}

// Document is the ledger with the pupil and the school, ready to print.
func (b *StatementBuilder) Document(ctx context.Context, studentID string, family bool) (StatementDoc, error) {
	ledger, err := b.Ledger(ctx, studentID, family)
	if err != nil {
		return StatementDoc{}, err
	}
	pupil, err := b.students.GetStudent(ctx, studentID)
	if err != nil {
		return StatementDoc{}, fmt.Errorf("reading the pupil of the statement: %w", err)
	}
	school, err := schoolOf(ctx, b.profiles)
	if err != nil {
		return StatementDoc{}, err
	}
	return StatementDoc{School: school, Pupil: pupil.FullName(), PupilDetail: pupil.GradeGroup, Statement: ledger}, nil
}
