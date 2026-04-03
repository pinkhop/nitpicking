package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// --- WithTransaction ---

func TestWithTransaction_ExecutesFnWithUnitOfWork(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()
	txn := memory.NewTransactor(repo)

	// Given — an issue stored in the repository.
	id := mustIssueID(t)
	now := time.Now()
	task := mustTask(t, id, "transactional issue", now)
	if err := repo.CreateIssue(ctx, task); err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — read the issue through the transactor's unit of work.
	var found bool
	err := txn.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		_, getErr := uow.Issues().GetIssue(ctx, id, false)
		found = getErr == nil
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !found {
		t.Error("expected to find the issue within the transaction")
	}
}

func TestWithTransaction_PropagatesFnError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()
	txn := memory.NewTransactor(repo)

	// Given
	sentinel := errors.New("deliberate failure")

	// When
	err := txn.WithTransaction(ctx, func(_ driven.UnitOfWork) error {
		return sentinel
	})

	// Then
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

// --- WithReadTransaction ---

func TestWithReadTransaction_ExecutesFnWithUnitOfWork(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()
	txn := memory.NewTransactor(repo)

	// Given — an issue stored in the repository.
	id := mustIssueID(t)
	now := time.Now()
	task := mustTask(t, id, "read-only issue", now)
	if err := repo.CreateIssue(ctx, task); err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When — read the issue through a read-only transaction.
	var found bool
	err := txn.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		_, getErr := uow.Issues().GetIssue(ctx, id, false)
		found = getErr == nil
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !found {
		t.Error("expected to find the issue within the read transaction")
	}
}

func TestWithReadTransaction_PropagatesFnError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()
	txn := memory.NewTransactor(repo)

	// Given
	sentinel := errors.New("read failure")

	// When
	err := txn.WithReadTransaction(ctx, func(_ driven.UnitOfWork) error {
		return sentinel
	})

	// Then
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

// --- WithTransaction UnitOfWork accessors ---

func TestWithTransaction_UnitOfWork_ExposesAllRepositories(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()
	txn := memory.NewTransactor(repo)

	// When — verify each accessor returns a non-nil repository.
	var (
		hasIssues        bool
		hasComments      bool
		hasClaims        bool
		hasRelationships bool
		hasHistory       bool
		hasDatabase      bool
	)
	err := txn.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		hasIssues = uow.Issues() != nil
		hasComments = uow.Comments() != nil
		hasClaims = uow.Claims() != nil
		hasRelationships = uow.Relationships() != nil
		hasHistory = uow.History() != nil
		hasDatabase = uow.Database() != nil
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !hasIssues {
		t.Error("UnitOfWork.Issues() returned nil")
	}
	if !hasComments {
		t.Error("UnitOfWork.Comments() returned nil")
	}
	if !hasClaims {
		t.Error("UnitOfWork.Claims() returned nil")
	}
	if !hasRelationships {
		t.Error("UnitOfWork.Relationships() returned nil")
	}
	if !hasHistory {
		t.Error("UnitOfWork.History() returned nil")
	}
	if !hasDatabase {
		t.Error("UnitOfWork.Database() returned nil")
	}
}

// --- Vacuum ---

func TestVacuum_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()
	txn := memory.NewTransactor(repo)

	// When
	err := txn.Vacuum(ctx)
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
