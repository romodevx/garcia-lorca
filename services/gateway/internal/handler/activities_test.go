package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

const (
	activityID  = "ac100000-0000-0000-0000-000000000001"
	actStudent  = "ac100000-0000-0000-0000-0000000000s1"
	actStudentB = "ac100000-0000-0000-0000-000000000052"
	actOption   = "ac100000-0000-0000-0000-0000000000a1"
)

type fakeActivities struct {
	input      client.ActivityInput
	updatedID  string
	items      []client.ActivityEnrollItem
	startsOn   string
	unenrolled []string
	endsOn     string
	option     string
	err        error
}

func (f *fakeActivities) ListActivities(context.Context, bool) ([]client.Activity, error) {
	return nil, f.err
}

func (f *fakeActivities) GetActivity(_ context.Context, id string) (client.Activity, error) {
	return client.Activity{ID: id}, f.err
}

func (f *fakeActivities) CreateActivity(_ context.Context, in client.ActivityInput) (client.Activity, error) {
	f.input = in
	return client.Activity{ID: activityID, ActivityInput: in}, f.err
}

func (f *fakeActivities) UpdateActivity(_ context.Context, id string, in client.ActivityInput) (client.Activity, error) {
	f.updatedID, f.input = id, in
	return client.Activity{ID: id, ActivityInput: in}, f.err
}

func (f *fakeActivities) ListActivityEnrollments(context.Context, string) ([]client.ActivityEnrollment, error) {
	return nil, f.err
}

func (f *fakeActivities) EnrollInActivity(_ context.Context, _ string, startsOn string, items []client.ActivityEnrollItem) ([]client.ActivityEnrollment, int32, error) {
	f.items, f.startsOn = items, startsOn
	return []client.ActivityEnrollment{{ID: "e-1", StudentID: actStudent}}, 3, f.err
}

func (f *fakeActivities) UnenrollFromActivity(_ context.Context, _ string, ids []string, endsOn string) (int32, error) {
	f.unenrolled, f.endsOn = ids, endsOn
	return int32(len(ids)), f.err
}

func (f *fakeActivities) ChangeActivityOption(_ context.Context, activityID, studentID, optionID string) (client.ActivityEnrollment, error) {
	f.option = optionID
	return client.ActivityEnrollment{ActivityID: activityID, StudentID: studentID, OptionID: optionID}, f.err
}

func activityMux(f *fakeActivities) *http.ServeMux {
	h := handler.NewActivitiesHandler(f)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/activities", h.List)
	mux.HandleFunc("POST /api/v1/activities", h.Create)
	mux.HandleFunc("GET /api/v1/activities/{id}", h.Get)
	mux.HandleFunc("PUT /api/v1/activities/{id}", h.Update)
	mux.HandleFunc("GET /api/v1/activities/{id}/enrollments", h.ListEnrollments)
	mux.HandleFunc("POST /api/v1/activities/{id}/enrollments", h.Enroll)
	mux.HandleFunc("POST /api/v1/activities/{id}/enrollments/end", h.Unenroll)
	mux.HandleFunc("PUT /api/v1/activities/{id}/enrollments/{studentId}", h.ChangeOption)
	return mux
}

func doActivity(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if s, ok := body.(string); ok {
			buf.WriteString(s)
		} else if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func swimBody() map[string]any {
	return map[string]any{
		"name":            " Natación ",
		"schedule":        []map[string]any{{"weekday": 2, "start": "15:00", "end": "16:30"}},
		"periodicity":     "monthly",
		"billingStartsOn": "2026-09-01",
		"firstDueOn":      "2026-09-10",
		"lateFeeCents":    5000,
		"active":          true,
		"options":         []map[string]any{{"daysPerWeek": 2, "amountCents": 70000}, {"daysPerWeek": nil, "amountCents": 90000}},
	}
}

func TestCreateActivityForwardsTheContract(t *testing.T) {
	f := &fakeActivities{}
	rec := doActivity(t, activityMux(f), http.MethodPost, "/api/v1/activities", swimBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	in := f.input
	if in.Name != "Natación" || in.Periodicity != "monthly" || in.BillingStartsOn != "2026-09-01" || in.FirstDueOn != "2026-09-10" ||
		in.LateFeeCents != 5000 || !in.Active || len(in.Schedule) != 1 || in.Schedule[0].End != "16:30" {
		t.Fatalf("forwarded = %+v", in)
	}
	if len(in.Options) != 2 || in.Options[0].DaysPerWeek == nil || *in.Options[0].DaysPerWeek != 2 || in.Options[1].DaysPerWeek != nil {
		t.Fatalf("options = %+v", in.Options)
	}
	if !strings.Contains(rec.Body.String(), `"id":"`+activityID+`"`) {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestActivityDefaultsToActiveAndNoLateFee(t *testing.T) {
	f := &fakeActivities{}
	body := swimBody()
	delete(body, "active")
	delete(body, "lateFeeCents")
	if rec := doActivity(t, activityMux(f), http.MethodPost, "/api/v1/activities", body); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if !f.input.Active || f.input.LateFeeCents != 0 {
		t.Fatalf("defaults = %+v", f.input)
	}
}

func TestCreateActivityRefusesBadInputAtTheEdge(t *testing.T) {
	cases := map[string]func(map[string]any){
		"blank name":  func(b map[string]any) { b["name"] = "  " },
		"long name":   func(b map[string]any) { b["name"] = strings.Repeat("a", 81) },
		"no schedule": func(b map[string]any) { b["schedule"] = []any{} },
		"weekday 8": func(b map[string]any) {
			b["schedule"] = []map[string]any{{"weekday": 8, "start": "15:00", "end": "16:00"}}
		},
		"bad clock": func(b map[string]any) {
			b["schedule"] = []map[string]any{{"weekday": 1, "start": "3pm", "end": "16:00"}}
		},
		"bad periodicity":   func(b map[string]any) { b["periodicity"] = "daily" },
		"bad date":          func(b map[string]any) { b["firstDueOn"] = "10/09/2026" },
		"negative late fee": func(b map[string]any) { b["lateFeeCents"] = -1 },
		"no options":        func(b map[string]any) { b["options"] = []any{} },
		"zero amount":       func(b map[string]any) { b["options"] = []map[string]any{{"daysPerWeek": 1, "amountCents": 0}} },
		"days 8":            func(b map[string]any) { b["options"] = []map[string]any{{"daysPerWeek": 8, "amountCents": 1}} },
		"bad option id": func(b map[string]any) {
			b["options"] = []map[string]any{{"id": "x", "daysPerWeek": 1, "amountCents": 1}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeActivities{}
			body := swimBody()
			mutate(body)
			if rec := doActivity(t, activityMux(f), http.MethodPost, "/api/v1/activities", body); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
		})
	}
	if rec := doActivity(t, activityMux(&fakeActivities{}), http.MethodPost, "/api/v1/activities", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: %d", rec.Code)
	}
}

func TestActivityRoutesNeedAnIDShapedID(t *testing.T) {
	mux := activityMux(&fakeActivities{})
	for _, path := range []string{"/api/v1/activities/nope", "/api/v1/activities/nope/enrollments"} {
		if rec := doActivity(t, mux, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, rec.Code)
		}
	}
}

func TestUpdateActivityForwardsTheID(t *testing.T) {
	f := &fakeActivities{}
	if rec := doActivity(t, activityMux(f), http.MethodPut, "/api/v1/activities/"+activityID, swimBody()); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if f.updatedID != activityID {
		t.Fatalf("id = %q", f.updatedID)
	}
}

func TestEnrollForwardsEachPupilsOption(t *testing.T) {
	f := &fakeActivities{}
	rec := doActivity(t, activityMux(f), http.MethodPost, "/api/v1/activities/"+activityID+"/enrollments", map[string]any{
		"startsOn":    "2026-09-02",
		"enrollments": []map[string]any{{"studentId": actStudentB, "optionId": actOption}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if len(f.items) != 1 || f.items[0].StudentID != actStudentB || f.items[0].OptionID != actOption || f.startsOn != "2026-09-02" {
		t.Fatalf("forwarded = %+v %q", f.items, f.startsOn)
	}
	if !strings.Contains(rec.Body.String(), `"chargesCreated":3`) {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestEnrollRefusesBadBatches(t *testing.T) {
	many := make([]map[string]any, 201)
	for i := range many {
		many[i] = map[string]any{"studentId": actStudentB, "optionId": actOption}
	}
	cases := map[string]map[string]any{
		"empty":        {"enrollments": []any{}},
		"too many":     {"enrollments": many},
		"bad student":  {"enrollments": []map[string]any{{"studentId": "x", "optionId": actOption}}},
		"bad option":   {"enrollments": []map[string]any{{"studentId": actStudentB, "optionId": "x"}}},
		"bad startsOn": {"startsOn": "mañana", "enrollments": []map[string]any{{"studentId": actStudentB, "optionId": actOption}}},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := doActivity(t, activityMux(&fakeActivities{}), http.MethodPost, "/api/v1/activities/"+activityID+"/enrollments", body); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestUnenrollAndChangeOption(t *testing.T) {
	f := &fakeActivities{}
	mux := activityMux(f)
	rec := doActivity(t, mux, http.MethodPost, "/api/v1/activities/"+activityID+"/enrollments/end",
		map[string]any{"studentIds": []string{actStudentB}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ended":1`) || f.endsOn != "" {
		t.Fatalf("unenroll = %d %s (endsOn %q)", rec.Code, rec.Body, f.endsOn)
	}
	if rec := doActivity(t, mux, http.MethodPost, "/api/v1/activities/"+activityID+"/enrollments/end",
		map[string]any{"studentIds": []string{"x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad student id = %d", rec.Code)
	}
	rec = doActivity(t, mux, http.MethodPut, "/api/v1/activities/"+activityID+"/enrollments/"+actStudentB,
		map[string]any{"optionId": actOption})
	if rec.Code != http.StatusOK || f.option != actOption {
		t.Fatalf("change option = %d %s", rec.Code, rec.Body)
	}
	if rec := doActivity(t, mux, http.MethodPut, "/api/v1/activities/"+activityID+"/enrollments/nope",
		map[string]any{"optionId": actOption}); rec.Code != http.StatusNotFound {
		t.Fatalf("bad student path = %d", rec.Code)
	}
}

func TestActivityRefusalsKeepTheirSentence(t *testing.T) {
	f := &fakeActivities{err: connect.NewError(connect.CodeAlreadyExists, errors.New("ya existe una actividad llamada Natación"))}
	rec := doActivity(t, activityMux(f), http.MethodPost, "/api/v1/activities", swimBody())
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "ya existe una actividad") {
		t.Fatalf("conflict = %d %s", rec.Code, rec.Body)
	}
}

func TestListActivitiesIsNeverNull(t *testing.T) {
	rec := doActivity(t, activityMux(&fakeActivities{}), http.MethodGet, "/api/v1/activities", nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"activities":[]}` {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
}
