package ingestion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/jobs"
)

type Service struct {
	repository       *PostgresRepository
	broker           *jobs.Broker
	processorVersion string
	now              func() time.Time
	logger           *slog.Logger
}

func NewService(repository *PostgresRepository, broker *jobs.Broker, logger *slog.Logger) (*Service, error) {
	if repository == nil {
		return nil, errors.New("ingestion repository is missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repository:       repository,
		broker:           broker,
		processorVersion: DefaultProcessor,
		now:              time.Now,
		logger:           logger,
	}, nil
}

// QueueBrochureExtraction is intentionally safe to call after the brochure
// row has been committed. The database job is durable and idempotent; a
// RabbitMQ outage changes the job to retrying rather than losing the upload.
func (s *Service) QueueBrochureExtraction(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode, storageKey, sha256Hex string) error {
	_, err := s.QueueBrochureExtractionWithID(ctx, adminID, academicYear, schoolCode, storageKey, sha256Hex)
	return err
}

// QueueBrochureExtractionWithID is the API-facing variant used by the manual
// admin upload path. Returning the durable ID lets the UI poll job status and
// retry a failed local extraction without uploading the PDF again.
func (s *Service) QueueBrochureExtractionWithID(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode, storageKey, sha256Hex string) (uuid.UUID, error) {
	return s.queueBrochureExtraction(ctx, &adminID, academicYear, schoolCode, storageKey, sha256Hex, "", "")
}

// QueueDiscoveredBrochureExtraction is the service-to-service entry point used
// after the authenticated discovery agent has stored a candidate PDF.
func (s *Service) QueueDiscoveredBrochureExtraction(ctx context.Context, academicYear int, schoolCode, storageKey, sha256Hex string) error {
	_, err := s.queueBrochureExtraction(ctx, nil, academicYear, schoolCode, storageKey, sha256Hex, "", "")
	return err
}

// QueueExternalBrochureExtraction is used by the authenticated HTTP
// ingestion adapter. It returns the durable job ID even when RabbitMQ is
// temporarily unavailable, so callers can poll and retry without uploading
// the source file again.
func (s *Service) QueueExternalBrochureExtraction(ctx context.Context, academicYear int, schoolCode, storageKey, sha256Hex, sourceURL string) (uuid.UUID, error) {
	return s.queueBrochureExtraction(ctx, nil, academicYear, schoolCode, storageKey, sha256Hex, sourceURL, "")
}

func (s *Service) QueueCandidateListExtraction(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode, programCode, storageKey, sha256Hex, sourceURL string) (uuid.UUID, error) {
	return s.queueCandidateListExtraction(ctx, &adminID, academicYear, schoolCode, programCode, storageKey, sha256Hex, sourceURL)
}

func (s *Service) QueueCandidateListExtractionSystem(ctx context.Context, academicYear int, schoolCode, programCode, storageKey, sha256Hex, sourceURL string) (uuid.UUID, error) {
	return s.queueCandidateListExtraction(ctx, nil, academicYear, schoolCode, programCode, storageKey, sha256Hex, sourceURL)
}

func (s *Service) queueBrochureExtraction(ctx context.Context, adminID *uuid.UUID, academicYear int, schoolCode, storageKey, sha256Hex, sourceURL, programCode string) (uuid.UUID, error) {
	if s == nil || s.repository == nil {
		return uuid.Nil, errors.New("ingestion service is not configured")
	}
	record, err := s.repository.queueDocumentJob(ctx, adminID, academicYear, strings.TrimSpace(schoolCode), storageKey, strings.ToLower(strings.TrimSpace(sha256Hex)), s.processorVersion, jobs.SourceTypeBrochure, sourceURL, programCode, s.now())
	if err != nil {
		return uuid.Nil, err
	}
	if !record.ShouldPublish || record.Status == "succeeded" {
		return record.Job.JobID, nil
	}
	if s.broker == nil {
		// The HTTP claim transport is a first-class alternative to RabbitMQ.
		// Leave the durable job queued so an external Python worker can claim it.
		return record.Job.JobID, nil
	}
	if err := s.broker.Publish(ctx, jobs.ExtractRoutingKey, record.Job.JobID, record.Job); err != nil {
		if markErr := s.repository.markDispatchFailed(ctx, record.Job.JobID, err); markErr != nil {
			s.logger.ErrorContext(ctx, "mark brochure extraction dispatch failure", "error", markErr)
		}
		return record.Job.JobID, DispatchError{err: err}
	}
	if err := s.repository.markDispatchStarted(ctx, record.Job.JobID); err != nil {
		// The message is already durable in RabbitMQ. The result consumer is
		// idempotent, so this metadata update can be repaired by a retry.
		s.logger.ErrorContext(ctx, "mark brochure extraction dispatch started", "error", err, "job_id", record.Job.JobID)
	}
	return record.Job.JobID, nil
}

func (s *Service) queueCandidateListExtraction(ctx context.Context, adminID *uuid.UUID, academicYear int, schoolCode, programCode, storageKey, sha256Hex, sourceURL string) (uuid.UUID, error) {
	if s == nil || s.repository == nil {
		return uuid.Nil, errors.New("ingestion service is not configured")
	}
	record, err := s.repository.queueDocumentJob(ctx, adminID, academicYear, strings.TrimSpace(schoolCode), storageKey, strings.ToLower(strings.TrimSpace(sha256Hex)), s.processorVersion, jobs.SourceTypeCandidateList, sourceURL, programCode, s.now())
	if err != nil {
		return uuid.Nil, err
	}
	if !record.ShouldPublish || record.Status == "succeeded" {
		return record.Job.JobID, nil
	}
	if s.broker == nil {
		// Candidate-list extraction can be consumed through the authenticated
		// HTTP claim API when RabbitMQ is intentionally not deployed.
		return record.Job.JobID, nil
	}
	if err := s.broker.Publish(ctx, jobs.CandidateListExtractRoutingKey, record.Job.JobID, record.Job); err != nil {
		if markErr := s.repository.markDispatchFailed(ctx, record.Job.JobID, err); markErr != nil {
			s.logger.ErrorContext(ctx, "mark candidate list dispatch failure", "error", markErr)
		}
		return record.Job.JobID, DispatchError{err: err}
	}
	if err := s.repository.markDispatchStarted(ctx, record.Job.JobID); err != nil {
		s.logger.ErrorContext(ctx, "mark candidate list dispatch started", "error", err, "job_id", record.Job.JobID)
	}
	return record.Job.JobID, nil
}

func (s *Service) RetryJob(ctx context.Context, adminID, jobID uuid.UUID) error {
	job, err := s.repository.RequeueJob(ctx, adminID, jobID)
	if err != nil {
		return err
	}
	if s.broker == nil {
		// The administrator can requeue a job even when the deployment uses the
		// HTTP claim transport instead of RabbitMQ.
		return nil
	}
	routingKey := jobs.ExtractRoutingKey
	if job.EffectiveSourceType() == jobs.SourceTypeCandidateList {
		routingKey = jobs.CandidateListExtractRoutingKey
	}
	if err := s.broker.Publish(ctx, routingKey, job.JobID, job); err != nil {
		if markErr := s.repository.markDispatchFailed(ctx, job.JobID, err); markErr != nil {
			s.logger.ErrorContext(ctx, "mark retry dispatch failure", "error", markErr, "job_id", job.JobID)
		}
		return DispatchError{err: err}
	}
	if err := s.repository.markDispatchStarted(ctx, job.JobID); err != nil {
		return fmt.Errorf("mark retry dispatch: %w", err)
	}
	return nil
}

type DispatchError struct{ err error }

func (e DispatchError) Error() string {
	if e.err == nil {
		return ErrDispatchUnavailable.Error()
	}
	return fmt.Sprintf("%s: %v", ErrDispatchUnavailable, e.err)
}

func (e DispatchError) Unwrap() error   { return ErrDispatchUnavailable }
func (e DispatchError) Retryable() bool { return true }
