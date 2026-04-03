package memory

import (
	"context"

	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// Transactor implements driven.Transactor using the in-memory repository.
// Since the repository is inherently atomic (protected by mutex), the
// transactor simply delegates to it.
type Transactor struct {
	repo *Repository
}

// NewTransactor creates a Transactor backed by the given repository.
func NewTransactor(repo *Repository) *Transactor {
	return &Transactor{repo: repo}
}

// WithTransaction executes fn with the repository as the unit of work.
// The in-memory adapter has no real transaction to commit/rollback.
func (t *Transactor) WithTransaction(_ context.Context, fn func(uow driven.UnitOfWork) error) error {
	uow := &unitOfWork{repo: t.repo}
	return fn(uow)
}

// WithReadTransaction executes fn with a read-only unit of work.
func (t *Transactor) WithReadTransaction(_ context.Context, fn func(uow driven.UnitOfWork) error) error {
	uow := &unitOfWork{repo: t.repo}
	return fn(uow)
}

// Vacuum is a no-op for the in-memory adapter — there is no disk to reclaim.
func (t *Transactor) Vacuum(_ context.Context) error {
	return nil
}

// unitOfWork wraps the repository to satisfy driven.UnitOfWork.
type unitOfWork struct {
	repo *Repository
}

func (u *unitOfWork) Issues() driven.IssueRepository               { return u.repo }
func (u *unitOfWork) Comments() driven.CommentRepository           { return u.repo }
func (u *unitOfWork) Claims() driven.ClaimRepository               { return u.repo }
func (u *unitOfWork) Relationships() driven.RelationshipRepository { return u.repo }
func (u *unitOfWork) History() driven.HistoryRepository            { return u.repo }
func (u *unitOfWork) Database() driven.DatabaseRepository          { return u.repo }
