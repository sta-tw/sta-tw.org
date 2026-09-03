package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"sta-backend/internal/jobs"
	"sta-backend/internal/results"
)

type ResultWorker struct {
	Repository  *PostgresRepository
	ListApplier CandidateListResultApplier
	Broker      *jobs.Broker
	Queue       string
	Consumer    string
	Logger      *slog.Logger
}

type CandidateListResultApplier interface {
	ApplyCandidateListExtractionResult(context.Context, jobs.CandidateListExtractionResult) error
}

func (w *ResultWorker) Run(ctx context.Context) error {
	if w == nil || w.Repository == nil || w.Broker == nil {
		return errors.New("ingestion result worker is not configured")
	}
	queue := w.Queue
	if queue == "" {
		queue = "sta.admissions.extracted"
	}
	consumer := w.Consumer
	if consumer == "" {
		consumer = "sta-api-ingestion-result"
	}
	deliveries, err := w.Broker.Consume(queue, consumer)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("ingestion result queue closed")
			}
			var envelope struct {
				ResultType string `json:"result_type"`
			}
			if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
				if w.Logger != nil {
					w.Logger.Warn("reject invalid extraction result")
				}
				_ = delivery.Reject(false)
				continue
			}
			var applyErr error
			var jobID string
			switch envelope.ResultType {
			case jobs.SourceTypeCandidateList:
				var result jobs.CandidateListExtractionResult
				if err := json.Unmarshal(delivery.Body, &result); err != nil || result.Validate() != nil || w.ListApplier == nil {
					if w.Logger != nil {
						w.Logger.Warn("reject invalid candidate list extraction result")
					}
					_ = delivery.Reject(false)
					continue
				}
				jobID = result.JobID.String()
				applyErr = w.ListApplier.ApplyCandidateListExtractionResult(ctx, result)
			default:
				var result jobs.BrochureExtractionResult
				if err := json.Unmarshal(delivery.Body, &result); err != nil || result.Validate() != nil {
					if w.Logger != nil {
						w.Logger.Warn("reject invalid brochure extraction result")
					}
					_ = delivery.Reject(false)
					continue
				}
				jobID = result.JobID.String()
				applyErr = w.Repository.ApplyExtractionResult(ctx, result)
			}
			if applyErr != nil {
				if errors.Is(applyErr, ErrInvalid) || errors.Is(applyErr, ErrNotFound) ||
					errors.Is(applyErr, results.ErrInvalidInput) || errors.Is(applyErr, results.ErrNotFound) ||
					errors.Is(applyErr, results.ErrConflict) || errors.Is(applyErr, results.ErrInvalidStatus) {
					if parsedJobID, parseErr := uuid.Parse(jobID); parseErr == nil {
						if markErr := w.Repository.MarkJobFailure(ctx, parsedJobID, JobFailureInput{
							Code: "result_rejected", Message: "extraction result was rejected", Retryable: false,
						}); markErr != nil && w.Logger != nil {
							w.Logger.Warn("could not mark rejected extraction result", "error", markErr, "job_id", jobID)
						}
					}
					if w.Logger != nil {
						w.Logger.Warn("reject unusable extraction result", "error", applyErr, "job_id", jobID)
					}
					_ = delivery.Reject(false)
					continue
				}
				if w.Logger != nil {
					w.Logger.Error("extraction result persistence failed", "error", applyErr, "job_id", jobID)
				}
				_ = delivery.Nack(false, true)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}
