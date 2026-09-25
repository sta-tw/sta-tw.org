package admissions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRepository struct {
	program Program
}

func (f fakeRepository) ListPrograms(context.Context, ProgramQuery) ([]Program, error) {
	return []Program{f.program}, nil
}

func (f fakeRepository) GetProgram(_ context.Context, identifier ProgramIdentifier) (Program, error) {
	if identifier.String() != f.program.ProgramIdentifier {
		return Program{}, ErrNotFound
	}
	return f.program, nil
}

func (f fakeRepository) ListSchools(context.Context, int) ([]School, error) {
	return []School{{SchoolCode: "001", SchoolName: "測試大學"}}, nil
}

func TestHandlerProgramRoutes(t *testing.T) {
	handler, err := NewHandler(fakeRepository{program: Program{ProgramIdentifier: "116-001-023"}})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admissions/programs/116-001-023", nil)
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admissions/programs/116-1-023", nil)
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid identifier status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestHandlerProgramQueryBounds(t *testing.T) {
	handler, _ := NewHandler(fakeRepository{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admissions/programs?limit=101", nil)
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestFakeRepositoryNotFound(t *testing.T) {
	repository := fakeRepository{program: Program{ProgramIdentifier: "116-001-023"}}
	_, err := repository.GetProgram(context.Background(), ProgramIdentifier{AcademicYear: 116, SchoolCode: "001", ProgramCode: "024"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
}
