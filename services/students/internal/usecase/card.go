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

// CardVerification is what a scanned code says: whose card it is, whether it
// still counts as a school credential (Valid: an active code of an enrolled
// pupil), and whether the office revoked it (Revoked: the card's own state,
// independent of admission).
type CardVerification struct {
	Student *domain.Student
	Valid   bool
	Revoked bool
}

// CardCodeResult is what the family's card shows: its code, or that the office
// suspended it.
type CardCodeResult struct {
	Code      string
	Suspended bool
}

// CardCode returns the code a pupil's card shows, issuing one the first time.
// A revoked card is suspended (the owner's decision): no new code is issued to
// whoever holds the family's session — a stolen phone would get a valid one —
// until the office reissues it (ReissueCard).
func (s *CardService) CardCode(ctx context.Context, studentID string) (CardCodeResult, error) {
	if _, err := s.pupils.FindByID(ctx, studentID); err != nil {
		return CardCodeResult{}, err
	}
	active, found, err := s.cards.ActiveCard(ctx, studentID)
	if err != nil {
		return CardCodeResult{}, err
	}
	if found {
		return CardCodeResult{Code: active.Code()}, nil
	}
	issuedBefore, err := s.cards.EverIssued(ctx, studentID)
	if err != nil {
		return CardCodeResult{}, err
	}
	if issuedBefore {
		return CardCodeResult{Suspended: true}, nil
	}
	code, err := s.issue(ctx, studentID)
	if err != nil {
		return CardCodeResult{}, err
	}
	return CardCodeResult{Code: code}, nil
}

// ReissueCard gives a suspended pupil a new card, on purpose, once the office
// trusts the phone again. A pupil whose card is valid already is KindConflict.
func (s *CardService) ReissueCard(ctx context.Context, studentID string) error {
	if _, err := s.pupils.FindByID(ctx, studentID); err != nil {
		return err
	}
	if _, found, err := s.cards.ActiveCard(ctx, studentID); err != nil {
		return err
	} else if found {
		return glerrors.New(glerrors.KindConflict, "the pupil's card is valid; revoke it first")
	}
	_, err := s.issue(ctx, studentID)
	return err
}

// issue mints and stores a new active code. Two first views at once: the
// loser shows the winner's code.
func (s *CardService) issue(ctx context.Context, studentID string) (string, error) {
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

// RevokeCard retires a pupil's current code and suspends the card until the
// office reissues it.
func (s *CardService) RevokeCard(ctx context.Context, studentID string) error {
	if _, err := s.pupils.FindByID(ctx, studentID); err != nil {
		return err
	}
	issuedBefore, err := s.cards.EverIssued(ctx, studentID)
	if err != nil {
		return err
	}
	if !issuedBefore {
		// No card yet (the family never opened it): a revoked one is stored,
		// so the suspension holds and the lost phone's first view gets no code.
		return s.suspendUnissued(ctx, studentID)
	}
	return s.cards.RevokeCards(ctx, studentID, s.now())
}

// suspendUnissued records a card that is born revoked.
func (s *CardService) suspendUnissued(ctx context.Context, studentID string) error {
	random, err := s.random()
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "minting a card code", err)
	}
	card, err := domain.NewCardToken(s.newID(), s.tenant, studentID, random, s.now())
	if err != nil {
		return err
	}
	if err := s.cards.SaveCard(ctx, card.Revoke(s.now())); err != nil {
		return err
	}
	// A first view racing this revoke may have issued an active card in the
	// meantime: retire it, so the suspension the office was told about holds.
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
	return CardVerification{
		Student: pupil,
		Valid:   card.Active() && pupil.Status() == domain.AdmissionEnrolled,
		Revoked: !card.Active(),
	}, nil
}
