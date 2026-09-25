package telegramcrosscheck

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"sta-backend/internal/results"
)

func TestChoiceLabelsAndInternalValues(t *testing.T) {
	tests := []struct {
		choice Choice
		label  string
		value  int16
	}{
		{ChoiceNotConsidering, "完全不考慮", 0},
		{ChoiceLowInterest, "意願偏低", 20},
		{ChoiceConsidering, "還在考慮", 40},
		{ChoiceLeaningYes, "傾向選擇", 60},
		{ChoiceHighInterest, "高度有意願", 80},
		{ChoiceDefinite, "確定選擇", 100},
	}
	for _, test := range tests {
		value, err := test.choice.InternalValue()
		if err != nil || value != test.value || test.choice.Label() != test.label {
			t.Fatalf("choice %q = value %d, label %q, error %v", test.choice, value, test.choice.Label(), err)
		}
		choice, err := ChoiceFromInternalValue(test.value)
		if err != nil || choice != test.choice {
			t.Fatalf("ChoiceFromInternalValue(%d) = %q, %v", test.value, choice, err)
		}
	}
}

func TestParticipantSyncValidation(t *testing.T) {
	valid := ParticipantSyncInput{
		Reason: "API integration test",
		Participants: []ParticipantInput{{
			TelegramUserID: 123456,
			Assignments: []AssignmentInput{{
				ProgramIdentifier: "116-001-001",
				CandidateNumber:   "TEST0001",
			}},
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
	invalid := valid
	invalid.Participants[0].Assignments[0].CandidateNumber = "ABC"
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate(short candidate number) error = nil")
	}
}

func TestServiceRespondMapsLabelToInternalValue(t *testing.T) {
	inquiryID := uuid.New()
	applicationID := uuid.New()
	accountID := uuid.New()
	repository := &fakeRepository{owner: InquiryOwner{
		AccountID: accountID, ApplicationID: applicationID,
		ProgramIdentifier: "116-001-001", SchoolName: "測試大學", ProgramName: "測試學系",
	}}
	writer := &fakeWillingnessWriter{}
	service, err := NewService(repository, writer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Respond(context.Background(), RespondInput{
		TelegramUserID: 123456,
		InquiryID:      inquiryID,
		Choice:         ChoiceLowInterest,
		CallbackID:     "callback-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if writer.value != 20 || writer.callbackID != "callback-1" || writer.inquiryID != inquiryID {
		t.Fatalf("writer call = value %d, inquiry %s, callback %q", writer.value, writer.inquiryID, writer.callbackID)
	}
	if result.ChoiceLabel != "意願偏低" || !repository.markedResponded {
		t.Fatalf("response = %#v, marked = %v", result, repository.markedResponded)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if _, leaked := payload["willingness"]; leaked {
		t.Fatalf("user-facing response leaked the internal willingness field: %s", encoded)
	}
	if _, leaked := payload["internal_value"]; leaked {
		t.Fatalf("user-facing response leaked the internal numeric value: %s", encoded)
	}
	if payload["choice"] != string(ChoiceLowInterest) || payload["choice_label"] != "意願偏低" {
		t.Fatalf("response JSON = %s", encoded)
	}
}

type fakeRepository struct {
	owner           InquiryOwner
	markedResponded bool
	bound           BindInput
}

func (repository *fakeRepository) IsAdmin(context.Context, uuid.UUID) (bool, error) { return true, nil }
func (repository *fakeRepository) AdminStatus(context.Context, uuid.UUID) (AdminStatus, error) {
	return AdminStatus{}, nil
}
func (repository *fakeRepository) SyncParticipants(context.Context, uuid.UUID, string, []PreparedParticipant) ([]ParticipantSyncResult, error) {
	return nil, nil
}
func (repository *fakeRepository) Bind(_ context.Context, input BindInput) error {
	repository.bound = input
	return nil
}
func (repository *fakeRepository) Disable(context.Context, int64) error { return nil }
func (repository *fakeRepository) Dashboard(context.Context, int64) (Dashboard, error) {
	return Dashboard{}, nil
}
func (repository *fakeRepository) History(context.Context, int64, int) ([]HistoryEvent, error) {
	return nil, nil
}
func (repository *fakeRepository) ResolveInquiry(context.Context, int64, uuid.UUID) (InquiryOwner, error) {
	if repository.owner.AccountID == uuid.Nil {
		return InquiryOwner{}, ErrNotFound
	}
	return repository.owner, nil
}
func (repository *fakeRepository) MarkResponded(context.Context, int64, uuid.UUID) error {
	repository.markedResponded = true
	return nil
}
func (repository *fakeRepository) ClaimDeliveries(context.Context, int) ([]Delivery, error) {
	return nil, nil
}
func (repository *fakeRepository) MarkSent(context.Context, uuid.UUID, int64) error { return nil }
func (repository *fakeRepository) MarkFailed(context.Context, uuid.UUID, string, bool) error {
	return nil
}

type fakeWillingnessWriter struct {
	value      int16
	inquiryID  uuid.UUID
	callbackID string
	err        error
}

func (writer *fakeWillingnessWriter) SetWillingnessFromChannel(_ context.Context, _, _ uuid.UUID, value int16, inquiryID uuid.UUID, _, callbackID string) (results.WillingnessResponse, error) {
	writer.value = value
	writer.inquiryID = inquiryID
	writer.callbackID = callbackID
	if writer.err != nil {
		return results.WillingnessResponse{}, writer.err
	}
	return results.WillingnessResponse{}, nil
}

var _ Repository = (*fakeRepository)(nil)
var _ WillingnessWriter = (*fakeWillingnessWriter)(nil)
var _ = errors.Is
