package verification

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/notifications"
	"sta-backend/internal/security"
	"sta-backend/internal/storage"
)

var threeDigitCode = regexp.MustCompile(`^[0-9]{3}$`)
var emailDomain = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
var verificationCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

type Service struct {
	repository         Repository
	cipher             *auth.FieldCipher
	lookupKey          []byte
	notifications      notifications.Repository
	blobStore          storage.BlobStore
	emailRateLimiter   *security.FixedWindowLimiter
	distributedLimiter security.DistributedLimiter
	now                func() time.Time
}

func (s *Service) ConfigureDistributedLimiter(limiter security.DistributedLimiter) {
	if s != nil {
		s.distributedLimiter = limiter
	}
}

func NewService(repository Repository, cipher *auth.FieldCipher, lookupKey []byte, notificationRepository notifications.Repository, blobStore storage.BlobStore) (*Service, error) {
	if repository == nil || cipher == nil || len(lookupKey) != 32 {
		return nil, errors.New("verification service dependencies are missing")
	}
	return &Service{
		repository: repository, cipher: cipher, lookupKey: append([]byte(nil), lookupKey...),
		notifications: notificationRepository, blobStore: blobStore,
		emailRateLimiter: security.NewFixedWindowLimiter(5, 10*time.Minute, 10000),
		now:              time.Now,
	}, nil
}

func (s *Service) CreateSchoolEmailRequest(ctx context.Context, accountID uuid.UUID, input CreateRequestInput) (Request, time.Time, error) {
	now := s.now().UTC()
	if s.emailRateLimiter != nil && !s.emailRateLimiter.Allow(accountID.String(), now) {
		return Request{}, time.Time{}, ErrConflict
	}
	if s.distributedLimiter != nil {
		allowed, err := s.distributedLimiter.Allow(ctx, "verification-email", accountID.String(), 5, 10*time.Minute, now)
		if err != nil {
			return Request{}, time.Time{}, err
		}
		if !allowed {
			return Request{}, time.Time{}, ErrConflict
		}
	}
	input, normalizedEmail, err := validateRequestInput(input, true)
	if err != nil {
		return Request{}, time.Time{}, err
	}
	allowed, err := s.repository.IsSchoolEmailAllowed(ctx, input.SchoolCode, emailDomainOf(normalizedEmail))
	if err != nil {
		return Request{}, time.Time{}, err
	}
	if !allowed {
		return Request{}, time.Time{}, ErrForbidden
	}
	emailCiphertext, err := s.cipher.Seal(normalizedEmail)
	if err != nil {
		return Request{}, time.Time{}, err
	}
	emailLookupHash, err := auth.LookupHash(s.lookupKey, normalizedEmail)
	if err != nil {
		return Request{}, time.Time{}, err
	}
	request, err := s.repository.CreateEmailRequest(ctx, accountID, input, emailCiphertext, emailLookupHash)
	if err != nil {
		return Request{}, time.Time{}, err
	}
	code, err := newVerificationCode()
	if err != nil {
		return Request{}, time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(10 * time.Minute)
	codeHash, err := auth.LookupHash(s.lookupKey, codeHashInput(request.ID, code))
	if err != nil {
		return Request{}, time.Time{}, err
	}
	if err := s.repository.CreateEmailChallenge(ctx, request.ID, codeHash, expiresAt); err != nil {
		return Request{}, time.Time{}, err
	}
	if s.notifications == nil {
		return Request{}, time.Time{}, errors.New("notification email delivery is not configured")
	}
	body := fmt.Sprintf("STA 學生身份驗證碼：%s\n此驗證碼 10 分鐘內有效，請勿轉交他人。", code)
	if err := s.notifications.EnqueueEmailTo(ctx, accountID, emailCiphertext, "verification-"+request.ID.String(), "STA 學生身份驗證碼", body); err != nil {
		return Request{}, time.Time{}, err
	}
	return request, expiresAt, nil
}

func (s *Service) VerifySchoolEmail(ctx context.Context, accountID, requestID uuid.UUID, code string) (Verification, error) {
	if !verificationCodePattern.MatchString(strings.TrimSpace(code)) {
		return Verification{}, ErrInvalidCode
	}
	now := s.now().UTC()
	expiresAt := now.AddDate(1, 0, 0)
	hash, err := auth.LookupHash(s.lookupKey, codeHashInput(requestID, strings.TrimSpace(code)))
	if err != nil {
		return Verification{}, err
	}
	return s.repository.ConsumeEmailCode(ctx, accountID, requestID, hash, now, expiresAt)
}

func (s *Service) CreateDocumentRequest(ctx context.Context, accountID uuid.UUID, input CreateRequestInput) (Request, error) {
	input, _, err := validateRequestInput(input, false)
	if err != nil {
		return Request{}, err
	}
	return s.repository.CreateDocumentRequest(ctx, accountID, input)
}

func (s *Service) ReviewDocumentRequest(ctx context.Context, adminID, requestID uuid.UUID, input ReviewInput) (ReviewResult, error) {
	if !input.Approved && strings.TrimSpace(input.Reason) == "" {
		return ReviewResult{}, ErrInvalid
	}
	if len(input.Reason) > 2000 {
		return ReviewResult{}, ErrInvalid
	}
	now := s.now().UTC()
	return s.repository.ReviewDocumentRequest(ctx, adminID, requestID, input, now, now.AddDate(1, 0, 0))
}

func (s *Service) AddDomain(ctx context.Context, adminID uuid.UUID, schoolCode, domain string) (Domain, error) {
	schoolCode = strings.TrimSpace(schoolCode)
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !threeDigitCode.MatchString(schoolCode) || !emailDomain.MatchString(domain) || strings.Contains(domain, "@") || len(domain) > 253 {
		return Domain{}, ErrInvalid
	}
	return s.repository.AddDomain(ctx, adminID, schoolCode, domain)
}

func (s *Service) PurgeAnnualData(ctx context.Context, academicYear int) (CleanupReport, error) {
	if s.blobStore == nil {
		return CleanupReport{}, errors.New("verification object storage is not configured")
	}
	return s.repository.PurgeAnnualData(ctx, academicYear, s.now().UTC(), s.blobStore.Remove)
}

func validateRequestInput(input CreateRequestInput, emailRequired bool) (CreateRequestInput, string, error) {
	input.SchoolCode = strings.TrimSpace(input.SchoolCode)
	input.ProgramCode = strings.TrimSpace(input.ProgramCode)
	input.SchoolEmail = auth.NormalizeEmail(input.SchoolEmail)
	if input.AcademicYear < 100 || input.AcademicYear > 999 || !threeDigitCode.MatchString(input.SchoolCode) || (input.ProgramCode != "" && !threeDigitCode.MatchString(input.ProgramCode)) {
		return CreateRequestInput{}, "", ErrInvalid
	}
	if emailRequired {
		parsed, err := mail.ParseAddress(input.SchoolEmail)
		if err != nil || parsed.Address != input.SchoolEmail || !strings.Contains(input.SchoolEmail, "@") {
			return CreateRequestInput{}, "", ErrInvalid
		}
		if len(input.SchoolEmail) > 320 {
			return CreateRequestInput{}, "", ErrInvalid
		}
		return input, input.SchoolEmail, nil
	}
	input.SchoolEmail = ""
	return input, "", nil
}

func emailDomainOf(value string) string {
	parts := strings.Split(value, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

func newVerificationCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func codeHashInput(requestID uuid.UUID, code string) string {
	return "student-verification:" + requestID.String() + ":" + code
}
