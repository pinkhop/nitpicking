package domain

import (
	"time"
	"unicode"
)

// Issue represents the core domain entity — either an Epic or a Task.
// Issues are immutable after construction; all mutation methods return a new
// Issue value. Revision and author are derived from history at read time,
// not stored on the struct.
type Issue struct {
	id                 ID
	role               Role
	title              string
	description        string
	acceptanceCriteria string
	priority           Priority
	state              State
	parentID           ID
	labels             LabelSet
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
	Labels             LabelSet
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
	Labels             LabelSet
	CreatedAt          time.Time
	IdempotencyKey     string
}

// NewTask creates a new task issue. The title must contain at least one
// alphanumeric character. The ID must be valid. Priority defaults to P2 if
// zero.
func NewTask(p NewTaskParams) (Issue, error) {
	if err := validateTitle(p.Title); err != nil {
		return Issue{}, err
	}
	if p.ID.IsZero() {
		return Issue{}, NewValidationError("id", "must not be empty")
	}

	priority := p.Priority
	if priority == 0 {
		priority = DefaultPriority
	}

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return Issue{
		id:                 p.ID,
		role:               RoleTask,
		title:              p.Title,
		description:        p.Description,
		acceptanceCriteria: p.AcceptanceCriteria,
		priority:           priority,
		state:              StateOpen,
		parentID:           p.ParentID,
		labels:             p.Labels,
		createdAt:          createdAt,
		idempotencyKey:     p.IdempotencyKey,
	}, nil
}

// NewEpic creates a new epic issue. The title must contain at least one
// alphanumeric character. The ID must be valid. Priority defaults to P2 if
// zero.
func NewEpic(p NewEpicParams) (Issue, error) {
	if err := validateTitle(p.Title); err != nil {
		return Issue{}, err
	}
	if p.ID.IsZero() {
		return Issue{}, NewValidationError("id", "must not be empty")
	}

	priority := p.Priority
	if priority == 0 {
		priority = DefaultPriority
	}

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return Issue{
		id:                 p.ID,
		role:               RoleEpic,
		title:              p.Title,
		description:        p.Description,
		acceptanceCriteria: p.AcceptanceCriteria,
		priority:           priority,
		state:              StateOpen,
		parentID:           p.ParentID,
		labels:             p.Labels,
		createdAt:          createdAt,
		idempotencyKey:     p.IdempotencyKey,
	}, nil
}

// Accessors — all return copies of the stored values.

// ID returns the issue's identifier.
func (t Issue) ID() ID { return t.id }

// Role returns the issue's role (task or epic).
func (t Issue) Role() Role { return t.role }

// Title returns the issue's title.
func (t Issue) Title() string { return t.title }

// Description returns the issue's description.
func (t Issue) Description() string { return t.description }

// AcceptanceCriteria returns the issue's acceptance criteria.
func (t Issue) AcceptanceCriteria() string { return t.acceptanceCriteria }

// Priority returns the issue's priority.
func (t Issue) Priority() Priority { return t.priority }

// State returns the issue's current lifecycle state.
func (t Issue) State() State { return t.state }

// ParentID returns the ID of the parent epic, or zero ID if unparented.
func (t Issue) ParentID() ID { return t.parentID }

// Labels returns the issue's label set.
func (t Issue) Labels() LabelSet { return t.labels }

// CreatedAt returns the issue's creation timestamp.
func (t Issue) CreatedAt() time.Time { return t.createdAt }

// IdempotencyKey returns the optional idempotency key used at creation.
func (t Issue) IdempotencyKey() string { return t.idempotencyKey }

// IsDeleted reports whether the issue has been soft-deleted.
func (t Issue) IsDeleted() bool { return t.deleted }

// IsTask reports whether the issue is a task.
func (t Issue) IsTask() bool { return t.role == RoleTask }

// IsEpic reports whether the issue is an epic.
func (t Issue) IsEpic() bool { return t.role == RoleEpic }

// Mutation methods — all return new Issue values.

// WithTitle returns a new issue with the updated title.
func (t Issue) WithTitle(title string) (Issue, error) {
	if err := validateTitle(title); err != nil {
		return Issue{}, err
	}
	t.title = title
	return t, nil
}

// WithDescription returns a new issue with the updated description.
func (t Issue) WithDescription(desc string) Issue {
	t.description = desc
	return t
}

// WithAcceptanceCriteria returns a new issue with the updated acceptance criteria.
func (t Issue) WithAcceptanceCriteria(ac string) Issue {
	t.acceptanceCriteria = ac
	return t
}

// WithPriority returns a new issue with the updated priority.
func (t Issue) WithPriority(p Priority) Issue {
	t.priority = p
	return t
}

// WithState returns a new issue with the updated state. This does not
// validate the transition — callers must use Transition before calling this.
func (t Issue) WithState(s State) Issue {
	t.state = s
	return t
}

// WithParentID returns a new issue with the updated parent epic ID. Pass a
// zero ID to remove the parent.
func (t Issue) WithParentID(parentID ID) Issue {
	t.parentID = parentID
	return t
}

// WithLabels returns a new issue with the updated label set.
func (t Issue) WithLabels(fs LabelSet) Issue {
	t.labels = fs
	return t
}

// WithDeleted returns a new issue marked as soft-deleted.
func (t Issue) WithDeleted() Issue {
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
	return NewValidationError("title", "must contain at least one alphanumeric character")
}
