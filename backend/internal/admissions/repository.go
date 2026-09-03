package admissions

import "context"

type Repository interface {
	ListPrograms(context.Context, ProgramQuery) ([]Program, error)
	GetProgram(context.Context, ProgramIdentifier) (Program, error)
	ListSchools(context.Context, int) ([]School, error)
}
