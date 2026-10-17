package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// MaxPlacements bounds one EnsurePlacements call (the proto's "at most 200").
const MaxPlacements = 200

// The stable per-placement error codes of EnsurePlacements (proto contract).
const (
	PlacementLevelNotFound     = "level_not_found"
	PlacementLevelBeingDeleted = "level_being_deleted"
	PlacementInvalid           = "invalid_placement"
)

// PlacementRequest asks for one level → grade → group of a cycle. LevelID
// names an existing level and wins when set; otherwise NewLevelName is matched
// by its folded name and created only when nothing matches.
type PlacementRequest struct {
	LevelID      string
	NewLevelName string
	GradeOrdinal int
	GroupName    string
	// Capacity is used only when the group is created, and must then be >= 1.
	Capacity int
}

// Placement answers one PlacementRequest. *Created is true for every
// placement that resolves to something this call created (or, in a dry run,
// would create); a dry run leaves the ids of those empty. Error, when set, is
// one of the Placement* codes and every other field is empty.
type Placement struct {
	LevelID      string
	LevelName    string
	LevelCreated bool
	GradeLevelID string
	GradeName    string
	GradeCreated bool
	GroupID      string
	GroupName    string
	GroupCreated bool
	// Capacity is the group's capacity after the call.
	Capacity int
	Error    string
}

// PlacementService finds or creates the catalog a legacy student import needs
// (ADR 0033): for each placement, its level, the level's grade by ordinal and
// the cycle's group of that grade by name.
type PlacementService struct {
	store  ports.PlacementStore
	tenant string
	newID  func() string
}

// NewPlacementService wires the use case; tenant and newID as in
// NewCatalogService.
func NewPlacementService(store ports.PlacementStore, tenant string, newID func() string) *PlacementService {
	return &PlacementService{store: store, tenant: tenant, newID: newID}
}

// EnsurePlacements answers every placement, in order, in ONE transaction.
//
// The call itself is refused (KindInvalid) for more than MaxPlacements or an
// unknown cycle; a placement that cannot be honoured is answered with its
// error code and does not affect the others. Each placement is resolved by
// lookups first and written only when it has no error, so a refused placement
// never leaves a half-built level behind. Identical placements — or several
// sharing a level or grade — resolve to the same rows, created once.
//
// With dryRun nothing is written: the answer says what WOULD be created.
//
// Concurrency: the inserts are find-or-create on unique keys (the folded level
// name, the grade ordinal, the group name), so two imports at once never
// duplicate a row; existing levels are share-locked for the call, so a level
// deletion cannot start underneath it and level_being_deleted is decided
// before anything is written. Any other fault (including a deadlock between
// two imports, KindConflict) rolls the whole call back.
func (s *PlacementService) EnsurePlacements(ctx context.Context, cycleID string, reqs []PlacementRequest, dryRun bool) ([]Placement, error) {
	if len(reqs) > MaxPlacements {
		return nil, glerrors.New(glerrors.KindInvalid, fmt.Sprintf("at most %d placements per call", MaxPlacements))
	}
	var out []Placement
	err := s.store.InPlacementTx(ctx, func(tx ports.PlacementTx) error {
		exists, err := tx.CycleExists(cycleID)
		if err != nil {
			return err
		}
		if !exists {
			return glerrors.New(glerrors.KindInvalid, "the cycle does not exist")
		}
		run := &placementRun{
			svc: s, tx: tx, cycleID: cycleID, dryRun: dryRun,
			levels: map[string]*levelRef{}, grades: map[string]*gradeRef{}, groups: map[string]*groupRef{},
		}
		answers := make([]Placement, 0, len(reqs))
		for _, req := range reqs {
			p, err := run.place(req)
			if err != nil {
				return err
			}
			answers = append(answers, p)
		}
		out = answers
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// levelRef, gradeRef and groupRef are a row as this call sees it. id is empty
// while the row is only planned (always, in a dry run). key identifies it for
// the rest of the call whether or not it is stored yet. A non-empty code means
// placements resolving to it cannot be honoured.
type levelRef struct {
	key, id, name string
	created       bool
	code          string
}

type gradeRef struct {
	key, id, name string
	ordinal       int
	created       bool
	code          string
}

type groupRef struct {
	key, id, name string
	capacity      int
	created       bool
	code          string
}

// placementRun is one call's state: the transaction and every row already
// resolved, so repeated placements answer alike and create once.
type placementRun struct {
	svc     *PlacementService
	tx      ports.PlacementTx
	cycleID string
	dryRun  bool
	// levels is keyed by "id:<id>" and "name:<folded name>"; both aliases of
	// one level point to the same ref.
	levels map[string]*levelRef
	grades map[string]*gradeRef
	groups map[string]*groupRef
}

func failed(code string) Placement { return Placement{Error: code} }

func (r *placementRun) place(req PlacementRequest) (Placement, error) {
	groupName := strings.ToUpper(strings.TrimSpace(req.GroupName))
	if req.GradeOrdinal < 1 || groupName == "" {
		return failed(PlacementInvalid), nil
	}
	level, err := r.level(req)
	if err != nil {
		return Placement{}, err
	}
	if level.code != "" {
		return failed(level.code), nil
	}
	grade, err := r.grade(level, req.GradeOrdinal)
	if err != nil {
		return Placement{}, err
	}
	if grade.code != "" {
		return failed(grade.code), nil
	}
	group, err := r.group(grade, groupName, req.Capacity)
	if err != nil {
		return Placement{}, err
	}
	if group.code != "" {
		return failed(group.code), nil
	}
	if !r.dryRun {
		code, err := r.store(level, grade, group)
		if err != nil {
			return Placement{}, err
		}
		if code != "" {
			return failed(code), nil
		}
	}
	return Placement{
		LevelID: level.id, LevelName: level.name, LevelCreated: level.created,
		GradeLevelID: grade.id, GradeName: grade.name, GradeCreated: grade.created,
		GroupID: group.id, GroupName: group.name, GroupCreated: group.created,
		Capacity: group.capacity,
	}, nil
}

// --- resolution (reads only) -------------------------------------------------

func (r *placementRun) level(req PlacementRequest) (*levelRef, error) {
	if req.LevelID != "" {
		return r.levelByID(req.LevelID)
	}
	// Capitals, as the office's own form writes a level (ISSUE-T14-008): an
	// imported "Maternal" beside a typed "PRIMARIA" read as two conventions.
	// Matching stays folded (case, accents, spacing), so a re-import never
	// duplicates a level whatever its capitals.
	name := strings.ToUpper(domain.CleanName(req.NewLevelName))
	if name == "" {
		return &levelRef{code: PlacementInvalid}, nil
	}
	return r.levelByName(name)
}

func (r *placementRun) levelByID(id string) (*levelRef, error) {
	if ref, ok := r.levels["id:"+id]; ok {
		return ref, nil
	}
	l, found, err := r.tx.LockLevel(id)
	if err != nil {
		return nil, err
	}
	if !found {
		ref := &levelRef{code: PlacementLevelNotFound}
		r.levels["id:"+id] = ref
		return ref, nil
	}
	return r.knownLevel(l), nil
}

func (r *placementRun) levelByName(name string) (*levelRef, error) {
	nameKey := "name:" + domain.FoldName(name)
	if ref, ok := r.levels[nameKey]; ok {
		return ref, nil
	}
	l, found, err := r.tx.LockLevelByName(name)
	if err != nil {
		return nil, err
	}
	if found {
		return r.knownLevel(l), nil
	}
	ref := &levelRef{key: "new:" + domain.FoldName(name), name: name, created: true}
	r.levels[nameKey] = ref
	return ref, nil
}

// knownLevel registers a stored level under both of its aliases, reusing the
// ref already registered under either.
func (r *placementRun) knownLevel(l domain.Level) *levelRef {
	idKey, nameKey := "id:"+l.ID(), "name:"+domain.FoldName(l.Name())
	ref, ok := r.levels[idKey]
	if !ok {
		ref = &levelRef{key: "lvl:" + l.ID(), id: l.ID(), name: l.Name()}
		if l.Deleting() {
			ref.code = PlacementLevelBeingDeleted
		}
	}
	r.levels[idKey], r.levels[nameKey] = ref, ref
	return ref
}

// gradeName is how a created grade is named: its ordinal and a degree sign.
func gradeName(ordinal int) string { return fmt.Sprintf("%d°", ordinal) }

func (r *placementRun) grade(level *levelRef, ordinal int) (*gradeRef, error) {
	key := fmt.Sprintf("%s/%d", level.key, ordinal)
	if ref, ok := r.grades[key]; ok {
		return ref, nil
	}
	ref := &gradeRef{key: key, name: gradeName(ordinal), ordinal: ordinal, created: true}
	if level.id != "" {
		g, found, err := r.tx.GradeByOrdinal(level.id, ordinal)
		if err != nil {
			return nil, err
		}
		if found {
			ref = &gradeRef{key: key, id: g.ID(), name: g.Name(), ordinal: ordinal}
		} else if taken, err := r.tx.GradeNameTaken(level.id, ref.name); err != nil {
			return nil, err
		} else if taken {
			// Reuse is by ordinal; a "<n>°" held at another ordinal is a
			// catalog the school has to straighten out first.
			ref.code = PlacementInvalid
		}
	}
	r.grades[key] = ref
	return ref, nil
}

func (r *placementRun) group(grade *gradeRef, name string, capacity int) (*groupRef, error) {
	key := grade.key + "/" + name
	if ref, ok := r.groups[key]; ok {
		return ref, nil
	}
	if grade.id != "" {
		g, found, err := r.tx.GroupByName(r.cycleID, grade.id, name)
		if err != nil {
			return nil, err
		}
		if found {
			ref := &groupRef{key: key, id: g.ID(), name: g.Name(), capacity: g.Capacity()}
			r.groups[key] = ref
			return ref, nil
		}
	}
	if capacity < 1 {
		// Not remembered: another placement may ask for the same group with a
		// usable capacity.
		return &groupRef{code: PlacementInvalid}, nil
	}
	ref := &groupRef{key: key, name: name, capacity: capacity, created: true}
	r.groups[key] = ref
	return ref, nil
}

// --- writes (commit only) ----------------------------------------------------

// store writes whatever of the placement is still only planned, top down. A
// row someone committed meanwhile is adopted (created=false). The code is set
// when what was found then cannot be honoured.
func (r *placementRun) store(level *levelRef, grade *gradeRef, group *groupRef) (string, error) {
	if level.id == "" {
		if err := r.storeLevel(level); err != nil || level.code != "" {
			return level.code, err
		}
	}
	if grade.id == "" {
		if err := r.storeGrade(level, grade); err != nil || grade.code != "" {
			return grade.code, err
		}
	}
	if group.id == "" {
		return "", r.storeGroup(grade, group)
	}
	return "", nil
}

func (r *placementRun) storeLevel(ref *levelRef) error {
	// The defaults of CreateLevel: per-cycle enrollment, no SEP code, and the
	// attendance and grading defaults the domain derives from the name.
	l, err := domain.NewLevel(r.svc.newID(), r.svc.tenant, ref.name, "", domain.GranularityCycle)
	if err != nil {
		return err
	}
	stored, created, err := r.tx.InsertLevel(l)
	if err != nil {
		return err
	}
	ref.id, ref.name, ref.created = stored.ID(), stored.Name(), created
	if stored.Deleting() {
		ref.code = PlacementLevelBeingDeleted
	}
	r.levels["id:"+stored.ID()] = ref
	return nil
}

func (r *placementRun) storeGrade(level *levelRef, ref *gradeRef) error {
	g, err := domain.NewGradeLevel(r.svc.newID(), r.svc.tenant, level.id, ref.name, ref.ordinal)
	if err != nil {
		return err
	}
	stored, created, err := r.tx.InsertGrade(g)
	if errors.Is(err, ports.ErrGradeNameTaken) {
		ref.code = PlacementInvalid
		return nil
	}
	if err != nil {
		return err
	}
	ref.id, ref.name, ref.created = stored.ID(), stored.Name(), created
	return nil
}

func (r *placementRun) storeGroup(grade *gradeRef, ref *groupRef) error {
	g, err := domain.NewGroup(r.svc.newID(), r.svc.tenant, r.cycleID, grade.id, ref.name, ref.capacity)
	if err != nil {
		return err
	}
	stored, created, err := r.tx.InsertGroup(g)
	if err != nil {
		return err
	}
	ref.id, ref.name, ref.capacity, ref.created = stored.ID(), stored.Name(), stored.Capacity(), created
	return nil
}
