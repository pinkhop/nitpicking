package fake

import (
	"context"

	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// Transactor implements port.Transactor using the fake repository.
// Since the in-memory repository is inherently atomic (protected by mutex),
// the transactor simply delegates to the repository.
type Transactor struct {
	repo *Repository
}

// NewTransactor creates a Transactor backed by the given repository.
func NewTransactor(repo *Repository) *Transactor {
	return &Transactor{repo: repo}
}

// WithTransaction executes fn with the repository as the unit of work.
// The fake has no real transaction to commit/rollback.
func (t *Transactor) WithTransaction(_ context.Context, fn func(uow port.UnitOfWork) error) error {
	uow := &unitOfWork{repo: t.repo}
	return fn(uow)
}

// WithReadTransaction executes fn with a read-only unit of work.
func (t *Transactor) WithReadTransaction(_ context.Context, fn func(uow port.UnitOfWork) error) error {
	uow := &unitOfWork{repo: t.repo}
	return fn(uow)
}

// unitOfWork wraps the repository to satisfy port.UnitOfWork.
type unitOfWork struct {
	repo *Repository
}

func (u *unitOfWork) Issues() port.IssueRepository               { return u.repo }
func (u *unitOfWork) Comments() port.CommentRepository           { return u.repo }
func (u *unitOfWork) Claims() port.ClaimRepository               { return u.repo }
func (u *unitOfWork) Relationships() port.RelationshipRepository { return u.repo }
func (u *unitOfWork) History() port.HistoryRepository            { return u.repo }
func (u *unitOfWork) Database() port.DatabaseRepository          { return u.repo }
