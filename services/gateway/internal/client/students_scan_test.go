package client

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
)

// fakeVerifyRPC implements only VerifyCard.
type fakeVerifyRPC struct {
	studentsv1connect.StudentsServiceClient
	res *studentsv1.VerifyCardResponse
}

func (f *fakeVerifyRPC) VerifyCard(_ context.Context, _ *connect.Request[studentsv1.VerifyCardRequest]) (*connect.Response[studentsv1.VerifyCardResponse], error) {
	return connect.NewResponse(f.res), nil
}

// ScanCard reads the card's own revocation, not the credential verdict: a
// pending pupil's active card is not valid, and it is not revoked either.
func TestScanCardReadsRevokedNotValid(t *testing.T) {
	tests := []struct {
		name        string
		valid       bool
		revoked     bool
		wantRevoked bool
	}{
		{name: "an active card of a pending pupil", valid: false, revoked: false, wantRevoked: false},
		{name: "a revoked card", valid: false, revoked: true, wantRevoked: true},
		{name: "an active card of an enrolled pupil", valid: true, revoked: false, wantRevoked: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &StudentsClient{rpc: &fakeVerifyRPC{res: &studentsv1.VerifyCardResponse{
				Student: &studentsv1.Student{Id: "stu-1"}, Valid: tc.valid, Revoked: tc.revoked,
			}}}
			got, err := c.ScanCard(context.Background(), "GLCARD:AAAAAAAAAAAAAAAAAAAAAA")
			if err != nil {
				t.Fatalf("ScanCard: %v", err)
			}
			if got.Student.ID != "stu-1" || got.Revoked != tc.wantRevoked {
				t.Errorf("ScanCard = %+v, want stu-1 revoked=%v", got, tc.wantRevoked)
			}
			// The credential check keeps its own verdict.
			_, valid, err := c.VerifyCard(context.Background(), "GLCARD:AAAAAAAAAAAAAAAAAAAAAA")
			if err != nil || valid != tc.valid {
				t.Errorf("VerifyCard valid = %v (%v), want %v", valid, err, tc.valid)
			}
		})
	}
}
