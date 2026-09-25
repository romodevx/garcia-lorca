package usecase

import (
	"context"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// PupilFinder reads a pupil by id.
type PupilFinder interface {
	FindByID(ctx context.Context, id string) (*domain.Student, error)
}

// CardService keeps the codes of the pupils' virtual cards (task 13): one active
// per pupil, revocable, and verified by the staff.
type CardService struct {
	cards  ports.CardRepository
	pupils PupilFinder
	tenant string
	newID  func() string
	random func() ([16]byte, error)
	now    func() time.Time
}

// NewCardService wires the service; random mints the 128 bits of a code.
func NewCardService(cards ports.CardRepository, pupils PupilFinder, tenant string, newID func() string, random func() ([16]byte, error), now func() time.Time) *CardService {
	return &CardService{cards: cards, pupils: pupils, tenant: tenant, newID: newID, random: random, now: now}
}

// CardVerification is what a scanned code says: whose card it is, and whether
// it still counts — an active code of an enrolled pupil.
type CardVerification struct {
	Student *domain.Student
	Valid   bool
}

// CardCode returns the code a pupil's card shows, issuing one the first time.
func (s *CardService) CardCode(ctx context.Context, studentID string) (string, error) {
	if _, err := s.pupils.FindByID(ctx, studentID); err != nil {
		return "", err
	}
	active, found, err := s.cards.ActiveCard(ctx, studentID)
	if err != nil {
		return "", err
	}
	if found {
		return active.Code(), nil
	}
	random, err := s.random()
	if err != nil {
		return "", glerrors.Wrap(glerrors.KindInternal, "minting a card code", err)
	}
	card, err := domain.NewCardToken(s.newID(), s.tenant, studentID, random, s.now())
	if err != nil {
		return "", err
	}
	if err := s.cards.SaveCard(ctx, card); err != nil {
		if glerrors.KindOf(err) != glerrors.KindConflict {
			return "", err
		}
		// Another first view issued it a moment ago: show that one.
		winner, found, rerr := s.cards.ActiveCard(ctx, studentID)
		if rerr != nil {
			return "", rerr
		}
		if !found {
			return "", err
		}
		return winner.Code(), nil
	}
	return card.Code(), nil
}

// RevokeCard retires a pupil's current code; the card shows a new one next.
func (s *CardService) RevokeCard(ctx context.Context, studentID string) error {
	if _, err := s.pupils.FindByID(ctx, studentID); err != nil {
		return err
	}
	return s.cards.RevokeCards(ctx, studentID, s.now())
}

// VerifyCard reads a scanned code: KindInvalid when it is not a card code,
// KindNotFound when no card ever carried it.
func (s *CardService) VerifyCard(ctx context.Context, code string) (CardVerification, error) {
	token, err := domain.ParseCardCode(code)
	if err != nil {
		return CardVerification{}, err
	}
	card, err := s.cards.FindCard(ctx, token)
	if err != nil {
		return CardVerification{}, err
	}
	pupil, err := s.pupils.FindByID(ctx, card.StudentID())
	if err != nil {
		return CardVerification{}, err
	}
	return CardVerification{Student: pupil, Valid: card.Active() && pupil.Status() == domain.AdmissionEnrolled}, nil
}
