package ticket

import (
	"time"
	"unicode"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// Ticket represents the core domain entity — either an Epic or a Task.
// Tickets are immutable after construction; all mutation methods return a new
// Ticket value. Revision and author are derived from history at read time,
// not stored on the struct.
type Ticket struct {
	id                 ID
	role               Role
	title              string
	description        string
	acceptanceCriteria string
	priority           Priority
	state              State
	parentID           ID
	facets             FacetSet
	createdAt          time.Time
	idempotencyKey     string
	deleted            bool
}

// NewTaskParams holds the required and optional parameters for creating a new task.
type NewTaskParams struct {
	ID                 ID
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           Priority
	ParentID           ID
	Facets             FacetSet
	CreatedAt          time.Time
	IdempotencyKey     string
}

// NewEpicParams holds the required and optional parameters for creating a new epic.
type NewEpicParams struct {
	ID                 ID
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           Priority
	ParentID           ID
	Facets             FacetSet
	CreatedAt          time.Time
	IdempotencyKey     string
}

// NewTask creates a new task ticket. The title must contain at least one
// alphanumeric character. The ID must be valid. Priority defaults to P2 if
// zero.
func NewTask(p NewTaskParams) (Ticket, error) {
	if err := validateTitle(p.Title); err != nil {
		return Ticket{}, err
	}
	if p.ID.IsZero() {
		return Ticket{}, domain.NewValidationError("id", "must not be empty")
	}

	priority := p.Priority
	if priority == 0 {
		priority = DefaultPriority
	}

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return Ticket{
		id:                 p.ID,
		role:               RoleTask,
		title:              p.Title,
		description:        p.Description,
		acceptanceCriteria: p.AcceptanceCriteria,
		priority:           priority,
		state:              StateOpen,
		parentID:           p.ParentID,
		facets:             p.Facets,
		createdAt:          createdAt,
		idempotencyKey:     p.IdempotencyKey,
	}, nil
}

// NewEpic creates a new epic ticket. The title must contain at least one
// alphanumeric character. The ID must be valid. Priority defaults to P2 if
// zero.
func NewEpic(p NewEpicParams) (Ticket, error) {
	if err := validateTitle(p.Title); err != nil {
		return Ticket{}, err
	}
	if p.ID.IsZero() {
		return Ticket{}, domain.NewValidationError("id", "must not be empty")
	}

	priority := p.Priority
	if priority == 0 {
		priority = DefaultPriority
	}

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return Ticket{
		id:                 p.ID,
		role:               RoleEpic,
		title:              p.Title,
		description:        p.Description,
		acceptanceCriteria: p.AcceptanceCriteria,
		priority:           priority,
		state:              StateActive,
		parentID:           p.ParentID,
		facets:             p.Facets,
		createdAt:          createdAt,
		idempotencyKey:     p.IdempotencyKey,
	}, nil
}

// Accessors — all return copies of the stored values.

// ID returns the ticket's identifier.
func (t Ticket) ID() ID { return t.id }

// Role returns the ticket's role (task or epic).
func (t Ticket) Role() Role { return t.role }

// Title returns the ticket's title.
func (t Ticket) Title() string { return t.title }

// Description returns the ticket's description.
func (t Ticket) Description() string { return t.description }

// AcceptanceCriteria returns the ticket's acceptance criteria.
func (t Ticket) AcceptanceCriteria() string { return t.acceptanceCriteria }

// Priority returns the ticket's priority.
func (t Ticket) Priority() Priority { return t.priority }

// State returns the ticket's current lifecycle state.
func (t Ticket) State() State { return t.state }

// ParentID returns the ID of the parent epic, or zero ID if unparented.
func (t Ticket) ParentID() ID { return t.parentID }

// Facets returns the ticket's facet set.
func (t Ticket) Facets() FacetSet { return t.facets }

// CreatedAt returns the ticket's creation timestamp.
func (t Ticket) CreatedAt() time.Time { return t.createdAt }

// IdempotencyKey returns the optional idempotency key used at creation.
func (t Ticket) IdempotencyKey() string { return t.idempotencyKey }

// IsDeleted reports whether the ticket has been soft-deleted.
func (t Ticket) IsDeleted() bool { return t.deleted }

// IsTask reports whether the ticket is a task.
func (t Ticket) IsTask() bool { return t.role == RoleTask }

// IsEpic reports whether the ticket is an epic.
func (t Ticket) IsEpic() bool { return t.role == RoleEpic }

// Mutation methods — all return new Ticket values.

// WithTitle returns a new ticket with the updated title.
func (t Ticket) WithTitle(title string) (Ticket, error) {
	if err := validateTitle(title); err != nil {
		return Ticket{}, err
	}
	t.title = title
	return t, nil
}

// WithDescription returns a new ticket with the updated description.
func (t Ticket) WithDescription(desc string) Ticket {
	t.description = desc
	return t
}

// WithAcceptanceCriteria returns a new ticket with the updated acceptance criteria.
func (t Ticket) WithAcceptanceCriteria(ac string) Ticket {
	t.acceptanceCriteria = ac
	return t
}

// WithPriority returns a new ticket with the updated priority.
func (t Ticket) WithPriority(p Priority) Ticket {
	t.priority = p
	return t
}

// WithState returns a new ticket with the updated state. This does not
// validate the transition — callers must use TransitionTask or TransitionEpic
// before calling this.
func (t Ticket) WithState(s State) Ticket {
	t.state = s
	return t
}

// WithParentID returns a new ticket with the updated parent epic ID. Pass a
// zero ID to remove the parent.
func (t Ticket) WithParentID(parentID ID) Ticket {
	t.parentID = parentID
	return t
}

// WithFacets returns a new ticket with the updated facet set.
func (t Ticket) WithFacets(fs FacetSet) Ticket {
	t.facets = fs
	return t
}

// WithDeleted returns a new ticket marked as soft-deleted.
func (t Ticket) WithDeleted() Ticket {
	t.deleted = true
	return t
}

// validateTitle checks that a title contains at least one alphanumeric
// character.
func validateTitle(title string) error {
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return nil
		}
	}
	return domain.NewValidationError("title", "must contain at least one alphanumeric character")
}
